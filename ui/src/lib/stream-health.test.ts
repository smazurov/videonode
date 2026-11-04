import { describe, expect, it } from 'vitest';
import {
  EARLY_DROP_FLOW,
  ENCODED_FLOW,
  FFMPEG_DROP_FLOW,
  FFMPEG_DUP_FLOW,
  REPEATED_FLOW,
  encoderActive,
  ffmpegStatsFresh,
  flowCounts,
  latestHealth,
  recentIncrease,
  silenceFlow,
  xrunFlow,
  type StreamHealth,
} from './stream-health';

function snapshot(state: StreamHealth['encoder']['state'] = 'running'): StreamHealth {
  return {
    sampled_at: '2026-06-01T00:00:00Z',
    stall_after_ms: 5000,
    publisher_started_at_us: 123,
    encoder: {
      state,
      started_at_us: 123,
      ffmpeg: {
        progress: { state: 'flowing', count: 10, age_ms: 20 },
        frames: { state: 'flowing', count: 300, age_ms: 20 },
        stats: { fps: 30, speed: 1, dropped_frames: 0, duplicate_frames: 0 },
      },
    },
    tracks: [],
  };
}

describe('stream health presentation', () => {
  it('requires reports and frame advancement to call FFmpeg stats fresh', () => {
    const health = snapshot();
    expect(ffmpegStatsFresh(health)).toBe(true);
    health.encoder.ffmpeg.frames.state = 'stalled';
    expect(ffmpegStatsFresh(health)).toBe(false);
    health.encoder.ffmpeg.frames.state = 'flowing';
    health.encoder.ffmpeg.progress.state = 'stalled';
    expect(ffmpegStatsFresh(health)).toBe(false);
  });

  it('does not classify idle processes as active', () => {
    for (const state of ['idle', 'stopping', 'error'] as const) {
      expect(encoderActive(snapshot(state))).toBe(false);
      expect(ffmpegStatsFresh(snapshot(state))).toBe(false);
    }
  });

  it('uses the newest REST or SSE snapshot, including restart/stop updates', () => {
    const older = snapshot();
    const newer = snapshot('idle');
    newer.sampled_at = '2026-06-01T00:00:01Z';
    expect(latestHealth(newer, older)).toBe(newer);
    expect(latestHealth(older, newer)).toBe(newer);
    expect(latestHealth(undefined, older)).toBe(older);
    expect(latestHealth(newer, undefined)).toBe(newer);
  });

  it('counts sink flows only once vn-sink has reported for this run', () => {
    const h = snapshot();
    const ffmpegOnly = { [ENCODED_FLOW]: 300, [FFMPEG_DUP_FLOW]: 0, [FFMPEG_DROP_FLOW]: 0 };
    expect(flowCounts(h)).toEqual(ffmpegOnly);
    h.encoder.sink = {
      reports: { state: 'flowing', count: 3, age_ms: 100 },
      video: { upstream: 'connected', repeated_frames: 8, dropped_frames: 1 },
      audio: [{ track: 0, device: 'hw:0,0', state: 'absent', xruns: 2, silence_ms: 9400, dropped_ms: 30 }],
    };
    expect(flowCounts(h)).toEqual({
      ...ffmpegOnly, [REPEATED_FLOW]: 8, [EARLY_DROP_FLOW]: 1,
      [silenceFlow(0)]: 9400, [xrunFlow(0)]: 2,
    });
    h.encoder.sink.reports = { state: 'never_received', count: 0 };
    expect(flowCounts(h)).toEqual(ffmpegOnly);
  });

  it('measures counter growth over the recent window only', () => {
    const at = (sec: number, n: number) => ({ ts: sec * 1000, counts: { x: n } });
    const steady = [at(0, 5), at(5, 5), at(9, 5), at(12, 7), at(20, 9)];
    expect(recentIncrease(steady, 'x')).toBe(4);
    expect(recentIncrease([at(0, 1), at(30, 1)], 'x')).toBe(0);
    expect(recentIncrease([at(0, 50), at(1, 3)], 'x')).toBe(3);
    expect(recentIncrease([at(0, 50), at(1, 3), at(2, 4)], 'x')).toBe(4);
    expect(recentIncrease([at(0, 1)], 'x')).toBe(0);
    expect(recentIncrease([at(0, 1), at(1, 4)], 'missing')).toBe(0);
  });
});
