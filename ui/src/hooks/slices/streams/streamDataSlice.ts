import { StateCreator } from 'zustand';

import type { Stream } from '../types';
import { StreamStore } from '../../useStreamStore';
import { assertNever, type EntityAction } from '../../entityTypes';

export interface StreamMetrics {
  fps?: string | undefined;
  dropped_frames?: string | undefined;
  duplicate_frames?: string | undefined;
  bytes_out?: number | undefined;
  packets_out?: number | undefined;
  [extra: string]: unknown;
}

export interface StreamDataSlice {
  streamIds: string[];
  streamsById: Record<string, Stream>;
  metricsById: Record<string, StreamMetrics>;
  // Live runtime slots populated by EntityEvent action=status|metrics|consumers.
  statusById: Record<string, unknown>;
  consumersById: Record<string, unknown>;
  streamRefreshKeys: Record<string, number>;

  setStreams: (streams: Stream[] | null | undefined) => void;
  addStream: (stream: Stream) => void;
  removeStream: (streamId: string) => void;
  bumpStreamRefreshKey: (streamId: string) => void;
  getStreamById: (streamId: string) => Stream | undefined;
  applyEntityEvent: (
    action: EntityAction,
    id: string,
    payload: unknown,
  ) => void;
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
  statusById: {},
  consumersById: {},
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

  bumpStreamRefreshKey: (streamId) => {
    set((state) => ({
      streamRefreshKeys: {
        ...state.streamRefreshKeys,
        [streamId]: (state.streamRefreshKeys[streamId] ?? 0) + 1,
      },
    }));
  },

  getStreamById: (streamId) => get().streamsById[streamId],

  applyEntityEvent: (action, id, payload) => {
    const { addStream, removeStream } = get();
    switch (action) {
      case 'created':
      case 'updated':
        if (payload) addStream(payload as Stream);
        return;
      case 'deleted':
        removeStream(id);
        return;
      case 'status':
        set((state) => ({
          statusById: { ...state.statusById, [id]: payload },
        }));
        return;
      case 'metrics':
        set((state) => ({
          metricsById: {
            ...state.metricsById,
            [id]: {
              ...(state.metricsById[id] ?? {}),
              ...(payload as Record<string, unknown>),
            },
          },
        }));
        return;
      case 'consumers':
        set((state) => ({
          consumersById: { ...state.consumersById, [id]: payload },
        }));
        return;
      default:
        assertNever(action);
    }
  },
});
