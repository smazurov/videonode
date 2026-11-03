import { describe, expect, it } from 'vitest';
import type { ActivityHealth, StreamHealth } from './stream-health';
import { encoderStage, relayStage, summarizeStream, viewersStage, type SourceLiveness } from './stream-status';

const flowing = (count: number): ActivityHealth => ({ state: 'flowing', count, age_ms: 10 });
const stalled = (count: number): ActivityHealth => ({ state: 'stalled', count, age_ms: 30_000 });

function health(state: StreamHealth['encoder']['state'] = 'running'): StreamHealth {
  return {
    sampled_at: '2026-06-01T00:00:00Z',
    stall_after_ms: 5000,
    publisher_started_at_us: 123,
    encoder: {
      state,
      started_at_us: 123,
      ffmpeg: {
        progress: flowing(10),
        frames: flowing(300),
        stats: { fps: 30, speed: 1, dropped_frames: 0, duplicate_frames: 0 },
      },
    },
    tracks: [
      { index: 0, type: 'video', codec: 'H264', packets: flowing(2000), units: flowing(300), relay_ms: 3 },
      { index: 1, type: 'audio', codec: 'opus', packets: flowing(500), units: flowing(500), relay_ms: 1 },
    ],
  };
}

const live: SourceLiveness = { id: 'lyra', live: true, label: 'Live' };
const noSignal: SourceLiveness = { id: 'lyra', live: false, label: 'No signal' };

describe('summarizeStream', () => {
  it('is healthy when everything flows', () => {
    expect(summarizeStream(true, health(), [live], undefined)).toEqual({ tone: 'ok', headline: 'Healthy' });
  });

  it('reports lost telemetry before guessing at health', () => {
    expect(summarizeStream(false, health('error'), [noSignal], undefined).headline).toBe('Telemetry disconnected');
  });

  it('explains an idle encoder as lazy, not broken', () => {
    expect(summarizeStream(true, health('idle'), [live], undefined)).toMatchObject({ tone: 'off', headline: 'Idle' });
    expect(summarizeStream(true, undefined, [], undefined)).toMatchObject({ tone: 'off', headline: 'Idle' });
  });

  it('surfaces a crashed encoder with the supervisor error', () => {
    expect(summarizeStream(true, health('error'), [live], 'process exited with code 1')).toEqual({
      tone: 'bad', headline: 'Encoder crashed', detail: 'process exited with code 1',
    });
  });

  it('names the first dead source before downstream symptoms', () => {
    const h = health();
    h.tracks[1] = { ...h.tracks[1]!, packets: stalled(500) };
    expect(summarizeStream(true, h, [noSignal], undefined)).toEqual({ tone: 'warn', headline: 'lyra: no signal' });
  });

  it('reports which track stopped and when', () => {
    const h = health();
    h.tracks[1] = { ...h.tracks[1]!, packets: stalled(500) };
    expect(summarizeStream(true, h, [live], undefined)).toEqual({ tone: 'bad', headline: 'Audio stopped 30s ago' });
  });

  it('warns when the relay holds frames before WebRTC', () => {
    const h = health();
    h.tracks[0] = { ...h.tracks[0]!, relay_ms: 350.4 };
    expect(summarizeStream(true, h, [live], undefined)).toEqual({ tone: 'warn', headline: 'Relay holding frames 350 ms' });
    expect(relayStage(h).tone).toBe('warn');
  });

  it('warns when the encoder cannot keep up', () => {
    const h = health();
    h.encoder.ffmpeg.stats.speed = 0.5;
    expect(summarizeStream(true, h, [live], undefined).headline).toBe("Encoder can't keep up (0.50×)");
  });
});

describe('pipeline stages', () => {
  it('shows encoder rate, idle, and crash', () => {
    expect(encoderStage(health(), 30)).toEqual({ state: '30 fps', tone: 'ok' });
    expect(encoderStage(health('idle'), null)).toEqual({ state: 'idle', tone: 'off' });
    expect(encoderStage(health('error'), null)).toEqual({ state: 'crashed', tone: 'bad' });
  });

  it('lists relay tracks and flags a stopped one', () => {
    expect(relayStage(health())).toEqual({ state: 'video + audio', tone: 'ok' });
    const h = health();
    h.tracks[1] = { ...h.tracks[1]!, packets: stalled(500) };
    expect(relayStage(h)).toEqual({ state: 'audio stopped', tone: 'bad' });
    expect(relayStage(health('idle'))).toEqual({ state: '—', tone: 'off' });
  });

  it('counts viewers, or says recording when only the recorder is reading', () => {
    expect(viewersStage(2, true)).toEqual({ state: '2', tone: 'ok' });
    expect(viewersStage(0, true)).toEqual({ state: 'recording', tone: 'ok' });
    expect(viewersStage(0, false)).toEqual({ state: '0', tone: 'off' });
  });
});
