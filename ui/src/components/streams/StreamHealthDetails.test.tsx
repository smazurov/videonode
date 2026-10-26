import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { StreamHealthDetails } from './StreamHealthDetails';
import type { StreamHealth } from '../../lib/stream-health';

function stalledHealth(): StreamHealth {
  const activity = { state: 'stalled' as const, count: 10, age_ms: 8000 };
  return {
    sampled_at: '2026-06-01T00:00:08Z', stall_after_ms: 5000, publisher_started_at_us: 123,
    encoder: {
      state: 'running', started_at_us: 123,
      ffmpeg: {
        progress: activity, frames: activity,
        stats: { fps: 30, speed: 1, dropped_frames: 0, duplicate_frames: 0 },
      },
    },
    tracks: [
      { index: 0, type: 'video', codec: 'H264', packets: activity, units: activity },
      { index: 1, type: 'audio', codec: 'opus', packets: activity, units: activity },
      { index: 2, type: 'audio', codec: 'opus', packets: activity, units: activity },
    ],
  };
}

describe('stream health details', () => {
  it('shows separate video and audio tracks', () => {
    const html = renderToStaticMarkup(<StreamHealthDetails health={stalledHealth()} connected />);
    expect(html).toContain('Video · H264');
    expect(html).toContain('Audio 1 · opus');
    expect(html).toContain('Audio 2 · opus');
    expect(html).toContain('8.0s ago');
    expect(html).toContain('Stalled');
  });

  it('shows telemetry disconnection rather than diagnosing a stall', () => {
    const html = renderToStaticMarkup(<StreamHealthDetails health={stalledHealth()} connected={false} />);
    expect(html).toContain('Telemetry disconnected');
    expect(html).not.toContain('Stalled');
  });

  it('does not show old media health when the encoder is idle', () => {
    const health = stalledHealth();
    health.encoder.state = 'idle';
    const html = renderToStaticMarkup(<StreamHealthDetails health={health} connected />);
    expect(html).toContain('Encoder is idle');
    expect(html).not.toContain('Packets received');
  });

  it('does not invent missing audio for video-only publishers', () => {
    const health = stalledHealth();
    health.tracks = health.tracks.filter((track) => track.type === 'video');
    const html = renderToStaticMarkup(<StreamHealthDetails health={health} connected />);
    expect(html).toContain('No audio track advertised');
  });
});
