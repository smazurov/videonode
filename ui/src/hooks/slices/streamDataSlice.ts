import { StateCreator } from 'zustand';
import type { components } from '../../lib/api.generated';
import { StreamStore } from '../useStreamStore';

type StreamData = components["schemas"]["StreamData"];
type StreamListData = components["schemas"]["StreamListData"];
type StreamMetricsEvent = components["schemas"]["StreamMetricsEvent"];

export interface StreamMetrics {
  fps?: string | undefined;
  dropped_frames?: string | undefined;
  duplicate_frames?: string | undefined;
}

export interface StreamDataSlice {
  streamIds: string[];
  streamsById: Record<string, StreamData>;
  metricsById: Record<string, StreamMetrics>;
  streamRefreshKeys: Record<string, number>;

  setStreams: (streamData: StreamListData) => void;
  addStream: (stream: StreamData) => void;
  removeStream: (streamId: string) => void;
  updateStreamMetrics: (metrics: StreamMetricsEvent) => void;
  bumpStreamRefreshKey: (streamId: string) => void;
  getStreamById: (streamId: string) => StreamData | undefined;
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

  setStreams: (streamData) => {
    const ids: string[] = [];
    const byId: Record<string, StreamData> = {};
    for (const stream of streamData.streams ?? []) {
      ids.push(stream.stream_id);
      byId[stream.stream_id] = stream;
    }
    set((state) => {
      // Preserve metrics for streams that still exist; only drop metrics for
      // deleted streams. Wiping wholesale flashes empty stats on every
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
      const isNew = !state.streamsById[stream.stream_id];
      return {
        streamIds: isNew
          ? [...state.streamIds, stream.stream_id]
          : state.streamIds,
        streamsById: { ...state.streamsById, [stream.stream_id]: stream },
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
        streamIds: state.streamIds.filter(id => id !== streamId),
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
