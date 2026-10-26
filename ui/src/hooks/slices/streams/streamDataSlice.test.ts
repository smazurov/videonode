import { beforeEach, describe, expect, it, vi } from 'vitest';
import { useStreamStore } from '../../useStreamStore';
import type { StreamEvent, StreamMetrics } from '../../entityTypes';

// These data-slice tests do not use HTTP or initialize the browser-only client.
vi.mock('../../../lib/api', () => ({ api: {}, unwrap: vi.fn() }));

function metricsEvent(second: number, run = 123, fresh = true): StreamEvent {
  const sampled = `2026-06-01T00:00:${String(second).padStart(2, '0')}Z`;
  const payload: StreamMetrics = {
    fps: 30, speed: 1, dropped_frames: 0, duplicate_frames: 0, bytes_out: second * 1000, packets_out: second * 10,
    health: {
      sampled_at: sampled, stall_after_ms: 5000, publisher_started_at_us: run, tracks: [],
      encoder: {
        state: 'running', started_at_us: run,
        ffmpeg: {
          progress: { state: fresh ? 'flowing' : 'stalled', count: 1 },
          frames: { state: fresh ? 'flowing' : 'stalled', count: 30 },
          stats: { fps: 30, speed: 1, dropped_frames: 0, duplicate_frames: 0 },
        },
      },
    },
  };
  return { type: 'stream.metrics', id: 's1', timestamp: sampled, payload };
}

beforeEach(() => {
  useStreamStore.setState({ metricsById: {}, metricsTsById: {}, metricsHistoryById: {}, streamsById: {}, streamIds: [] });
});

describe('stream health in metrics history', () => {
  it('does not chart stale FPS as fresh samples', () => {
    const apply = useStreamStore.getState().applyEntityEvent;
    apply(metricsEvent(1));
    apply(metricsEvent(2, 123, false));
    expect(useStreamStore.getState().metricsHistoryById.s1?.map((sample) => sample.fps)).toEqual([30, null]);
  });

  it('resets history when the encoder run changes', () => {
    const apply = useStreamStore.getState().applyEntityEvent;
    apply(metricsEvent(1));
    apply(metricsEvent(2, 456));
    const history = useStreamStore.getState().metricsHistoryById.s1;
    expect(history).toHaveLength(1);
    expect(history?.[0]?.bitrateBps).toBeNull();
  });

  it('ignores delayed older SSE snapshots', () => {
    const apply = useStreamStore.getState().applyEntityEvent;
    apply(metricsEvent(2, 456));
    apply(metricsEvent(1, 123));
    expect(useStreamStore.getState().metricsById.s1?.health?.encoder.started_at_us).toBe(456);
    expect(useStreamStore.getState().metricsHistoryById.s1).toHaveLength(1);
  });

  it('does not replace a newer REST snapshot with old SSE', () => {
    const newer = metricsEvent(3, 456);
    if (newer.type !== 'stream.metrics' || !newer.payload.health) throw new Error('fixture');
    useStreamStore.getState().addStream({
      stream_id: 's1', upstream: 'source:cam', enabled: true, health: newer.payload.health,
    });
    useStreamStore.getState().applyEntityEvent(metricsEvent(2, 123));
    expect(useStreamStore.getState().metricsById.s1).toBeUndefined();
  });
});
