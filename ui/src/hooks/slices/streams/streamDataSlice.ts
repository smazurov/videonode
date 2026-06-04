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

// Rolling window of per-stream metrics samples kept in the store so history
// accumulates app-wide from the SSE stream, independent of which view is
// mounted, and survives navigation. One sample per ~1s push, so this doubles
// as the max time span (seconds) the sparklines can show.
export const METRICS_HISTORY_LENGTH = 30;

export interface MetricsSample {
  readonly ts: number;
  readonly fps: number | null;
  readonly bytesOut: number | null;
  // Throughput derived from consecutive bytes_out/ts deltas (config bitrate is
  // static, so it can't be trended).
  readonly bitrateBps: number | null;
}

export interface StreamDataSlice {
  streamIds: string[];
  streamsById: Record<string, Stream>;
  metricsById: Record<string, StreamMetrics>;
  // Wall-clock ms of the most recent stream.metrics push, keyed by id, so
  // uptime advances in lockstep with the metrics instead of wall-clock now.
  metricsTsById: Record<string, number>;
  metricsHistoryById: Record<string, MetricsSample[]>;
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
  metricsTsById: {},
  metricsHistoryById: {},
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
      const nextMetricsTs: Record<string, number> = {};
      const nextHistory: Record<string, MetricsSample[]> = {};
      for (const id of ids) {
        if (state.metricsById[id]) nextMetrics[id] = state.metricsById[id];
        if (state.metricsTsById[id]) nextMetricsTs[id] = state.metricsTsById[id];
        if (state.metricsHistoryById[id]) nextHistory[id] = state.metricsHistoryById[id];
      }
      return {
        streamIds: ids,
        streamsById: byId,
        metricsById: nextMetrics,
        metricsTsById: nextMetricsTs,
        metricsHistoryById: nextHistory,
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
      const restStreams = { ...state.streamsById };
      const restMetrics = { ...state.metricsById };
      const restMetricsTs = { ...state.metricsTsById };
      const restHistory = { ...state.metricsHistoryById };
      delete restStreams[streamId];
      delete restMetrics[streamId];
      delete restMetricsTs[streamId];
      delete restHistory[streamId];
      return {
        streamIds: state.streamIds.filter((id) => id !== streamId),
        streamsById: restStreams,
        metricsById: restMetrics,
        metricsTsById: restMetricsTs,
        metricsHistoryById: restHistory,
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
      case 'stream.metrics': {
        const parsed = Date.parse(event.timestamp);
        const ts = Number.isNaN(parsed) ? Date.now() : parsed;
        set((state) => {
          const payload = event.payload;
          const fps = Number.isFinite(payload.fps) ? payload.fps : null;
          const bytesOut = Number.isFinite(payload.bytes_out) ? payload.bytes_out : null;
          const prevHistory = state.metricsHistoryById[event.id] ?? [];
          const last = prevHistory[prevHistory.length - 1];
          let bitrateBps: number | null = null;
          if (last && bytesOut != null && last.bytesOut != null && ts > last.ts) {
            const dtSec = (ts - last.ts) / 1000;
            const dBytes = bytesOut - last.bytesOut;
            if (dtSec > 0 && dBytes >= 0) bitrateBps = (dBytes * 8) / dtSec;
          }
          const nextHistory = [...prevHistory, { ts, fps, bytesOut, bitrateBps }];
          if (nextHistory.length > METRICS_HISTORY_LENGTH) nextHistory.shift();
          return {
            metricsById: { ...state.metricsById, [event.id]: payload },
            metricsTsById: { ...state.metricsTsById, [event.id]: ts },
            metricsHistoryById: { ...state.metricsHistoryById, [event.id]: nextHistory },
          };
        });
        return;
      }
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
