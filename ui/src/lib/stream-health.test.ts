import { describe, expect, it } from 'vitest';
import { activityLabel, encoderActive, ffmpegStatsFresh, latestHealth, type StreamHealth } from './stream-health';

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

  it('distinguishes missing data, stalled data, and unsupported depacketization', () => {
    expect(activityLabel({ state: 'never_received', count: 0 }).label).toBe('Waiting');
    expect(activityLabel({ state: 'stalled', count: 4, age_ms: 8100 })).toMatchObject({
      label: 'Stalled', tone: 'error', detail: 'Last arrival 8.1s ago',
    });
    expect(activityLabel({ state: 'unsupported', count: 0 }).label).toBe('Packet-only');
  });
});
