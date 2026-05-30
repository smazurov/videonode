import { StateCreator } from 'zustand';

import type { Stream } from '../types';
import { StreamStore } from '../../useStreamStore';
import {
  assertNever,
  type StreamConsumers,
  type StreamEvent,
  type StreamMetrics,
  type StreamStatus,
} from '../../entityTypes';

export type { StreamMetrics };

export interface StreamDataSlice {
  streamIds: string[];
  streamsById: Record<string, Stream>;
  metricsById: Record<string, StreamMetrics>;
  // Live runtime slots populated by stream.status|metrics|consumers events.
  statusById: Record<string, StreamStatus>;
  consumersById: Record<string, StreamConsumers>;
  streamRefreshKeys: Record<string, number>;

  setStreams: (streams: Stream[] | null | undefined) => void;
  addStream: (stream: Stream) => void;
  removeStream: (streamId: string) => void;
  bumpStreamRefreshKey: (streamId: string) => void;
  getStreamById: (streamId: string) => Stream | undefined;
  applyEntityEvent: (event: StreamEvent) => void;
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

  applyEntityEvent: (event) => {
    const { addStream, removeStream } = get();
    switch (event.type) {
      case 'stream.created':
      case 'stream.updated':
        addStream(event.payload);
        return;
      case 'stream.deleted':
        removeStream(event.id);
        return;
      case 'stream.status':
        set((state) => ({
          statusById: { ...state.statusById, [event.id]: event.payload },
        }));
        return;
      case 'stream.metrics':
        set((state) => ({
          metricsById: { ...state.metricsById, [event.id]: event.payload },
        }));
        return;
      case 'stream.consumers':
        set((state) => ({
          consumersById: { ...state.consumersById, [event.id]: event.payload },
        }));
        return;
      default:
        assertNever(event);
    }
  },
});
