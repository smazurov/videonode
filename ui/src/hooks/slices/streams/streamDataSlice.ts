import { StateCreator } from 'zustand';

import type { components } from '../../../lib/api.generated';
import type { Stream } from '../types';
import { StreamStore } from '../../useStreamStore';

type StreamMetricsEvent = components['schemas']['StreamMetricsEvent'];

export interface StreamMetrics {
  fps?: string | undefined;
  dropped_frames?: string | undefined;
  duplicate_frames?: string | undefined;
}

export interface StreamDataSlice {
  streamIds: string[];
  streamsById: Record<string, Stream>;
  metricsById: Record<string, StreamMetrics>;
  streamRefreshKeys: Record<string, number>;

  setStreams: (streams: Stream[] | null | undefined) => void;
  addStream: (stream: Stream) => void;
  removeStream: (streamId: string) => void;
  updateStreamMetrics: (metrics: StreamMetricsEvent) => void;
  bumpStreamRefreshKey: (streamId: string) => void;
  getStreamById: (streamId: string) => Stream | undefined;
}

// Alphabetical by id — stable grid order across refetches + SSE addStream.
function sortStreamIds(ids: string[]): string[] {
  return [...ids].sort((a, b) => a.localeCompare(b));
}

export const createStreamDataSlice: StateCreator<
  StreamStore,
  [],
  [],
  StreamDataSlice
> = (set, get) => ({
  streamIds: [],
  streamsById: {},
  metricsById: {},
  streamRefreshKeys: {},

  setStreams: (streams) => {
    const byId: Record<string, Stream> = {};
    for (const stream of streams ?? []) {
      byId[stream.stream_id] = stream;
    }
    const ids = sortStreamIds(Object.keys(byId));
    set((state) => {
      // Preserve metrics for streams that still exist; only drop metrics
      // for deleted streams. Wiping wholesale flashes empty stats on every
      // refresh until SSE refills them.
      const nextMetrics: Record<string, StreamMetrics> = {};
      for (const id of ids) {
        if (state.metricsById[id]) nextMetrics[id] = state.metricsById[id];
      }
      return {
        streamIds: ids,
        streamsById: byId,
        metricsById: nextMetrics,
        lastUpdated: new Date(),
      };
    });
  },

  addStream: (stream) => {
    set((state) => {
      const existed = !!state.streamsById[stream.stream_id];
      const streamsById = { ...state.streamsById, [stream.stream_id]: stream };
      const nextIds = existed
        ? sortStreamIds(state.streamIds)
        : sortStreamIds([...state.streamIds, stream.stream_id]);
      return {
        streamIds: nextIds,
        streamsById,
        lastUpdated: new Date(),
      };
    });
  },

  removeStream: (streamId) => {
    set((state) => {
      // eslint-disable-next-line @typescript-eslint/no-unused-vars, sonarjs/no-unused-vars
      const { [streamId]: _, ...restStreams } = state.streamsById;
      // eslint-disable-next-line @typescript-eslint/no-unused-vars, sonarjs/no-unused-vars
      const { [streamId]: __, ...restMetrics } = state.metricsById;
      return {
        streamIds: state.streamIds.filter((id) => id !== streamId),
        streamsById: restStreams,
        metricsById: restMetrics,
        lastUpdated: new Date(),
      };
    });
  },

  updateStreamMetrics: (metrics) => {
    set((state) => {
      const existing = state.metricsById[metrics.stream_id];
      // No-op when the three reported fields are identical — avoids
      // re-rendering every consumer on each metrics tick.
      if (
        existing &&
        existing.fps === metrics.fps &&
        existing.dropped_frames === metrics.dropped_frames &&
        existing.duplicate_frames === metrics.duplicate_frames
      ) {
        return state;
      }
      return {
        metricsById: {
          ...state.metricsById,
          [metrics.stream_id]: {
            ...existing,
            fps: metrics.fps,
            dropped_frames: metrics.dropped_frames,
            duplicate_frames: metrics.duplicate_frames,
          },
        },
      };
    });
  },

  bumpStreamRefreshKey: (streamId) => {
    set((state) => ({
      streamRefreshKeys: {
        ...state.streamRefreshKeys,
        [streamId]: (state.streamRefreshKeys[streamId] ?? 0) + 1,
      },
    }));
  },

  getStreamById: (streamId) => get().streamsById[streamId],
});
