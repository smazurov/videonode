import { StateCreator } from 'zustand';

import type { Source } from '../types';
import { SourceStore } from '../../useSourceStore';
import {
  assertNever,
  type SourceConsumers,
  type SourceEvent,
  type StatusParams,
} from '../../entityTypes';

export interface SourceDataSlice {
  sourceIds: string[];
  sourcesById: Record<string, Source>;
  statusById: Record<string, StatusParams>;
  consumersById: Record<string, SourceConsumers>;

  setSources: (sources: Source[] | null | undefined) => void;
  addSource: (source: Source) => void;
  removeSource: (sourceId: string) => void;
  getSourceById: (sourceId: string) => Source | undefined;
  applyEntityEvent: (event: SourceEvent) => void;
}

function sortIds(ids: string[]): string[] {
  return [...ids].sort((a, b) => a.localeCompare(b));
}

export const createSourceDataSlice: StateCreator<
  SourceStore,
  [],
  [],
  SourceDataSlice
> = (set, get) => ({
  sourceIds: [],
  sourcesById: {},
  statusById: {},
  consumersById: {},

  setSources: (sources) => {
    set((state) => {
      const byId: Record<string, Source> = {};
      for (const source of sources ?? []) {
        byId[source.id] = mergeRuntime(state.sourcesById[source.id], source);
      }
      return {
        sourceIds: sortIds(Object.keys(byId)),
        sourcesById: byId,
        lastUpdated: new Date(),
      };
    });
  },

  addSource: (source) => {
    set((state) => {
      const existed = !!state.sourcesById[source.id];
      const merged = mergeRuntime(state.sourcesById[source.id], source);
      const sourcesById = { ...state.sourcesById, [source.id]: merged };
      const nextIds = existed
        ? sortIds(state.sourceIds)
        : sortIds([...state.sourceIds, source.id]);
      return {
        sourceIds: nextIds,
        sourcesById,
        lastUpdated: new Date(),
      };
    });
  },

  removeSource: (sourceId) => {
    set((state) => {
      const rest = { ...state.sourcesById };
      delete rest[sourceId];
      return {
        sourceIds: state.sourceIds.filter((id) => id !== sourceId),
        sourcesById: rest,
        lastUpdated: new Date(),
      };
    });
  },

  getSourceById: (sourceId) => get().sourcesById[sourceId],

  applyEntityEvent: (event) => {
    const { addSource, removeSource } = get();
    switch (event.type) {
      case 'source.created':
      case 'source.updated':
        addSource(event.payload);
        return;
      case 'source.deleted':
        removeSource(event.id);
        return;
      case 'source.status': {
        const { id, payload } = event;
        set((state) => {
          const next = { ...state.statusById, [id]: payload };
          const src = state.sourcesById[id];
          if (!src) return { statusById: next };
          const lastAt = payload.ts_ms
            ? new Date(payload.ts_ms).toISOString()
            : new Date().toISOString();
          const merged: Source = { ...src, last_status_at: lastAt, latest_status: payload };
          if (isLivenessToken(payload.health)) merged.liveness = payload.health;
          if (payload.started_at_us !== undefined && payload.started_at_us > 0) {
            merged.started_at_us = payload.started_at_us;
          } else {
            delete merged.started_at_us;
          }
          const fps = computeEffectiveFps(state.statusById[id], payload);
          if (fps !== undefined) merged.effective_fps = fps;
          else delete merged.effective_fps;
          return {
            statusById: next,
            sourcesById: { ...state.sourcesById, [id]: merged },
          };
        });
        return;
      }
      case 'source.consumers': {
        const { id, payload } = event;
        set((state) => {
          const next = { ...state.consumersById, [id]: payload };
          const src = state.sourcesById[id];
          if (!src) return { consumersById: next };
          return {
            consumersById: next,
            sourcesById: {
              ...state.sourcesById,
              [id]: { ...src, consumer_count: payload.count },
            },
          };
        });
        return;
      }
      default:
        assertNever(event);
    }
  },
});

const LIVENESS_TOKENS = new Set<string>([
  'live',
  'transitioning',
  'no_cable',
  'no_signal',
  'initializing',
  'offline',
  'unknown',
]);

// The wire `health` token is a free string (it can be "idle"); validate it
// against SourceData.liveness's enum before lifting it onto the list-row field.
function isLivenessToken(value: string): value is NonNullable<Source['liveness']> {
  return LIVENESS_TOKENS.has(value);
}

function mergeRuntime(prev: Source | undefined, next: Source): Source {
  if (!prev) return next;
  const merged: Source = { ...next };
  if (prev.latest_status !== undefined) merged.latest_status = prev.latest_status;
  if (prev.last_status_at !== undefined) merged.last_status_at = prev.last_status_at;
  if (prev.started_at_us !== undefined && next.status !== 'idle') merged.started_at_us = prev.started_at_us;
  if (prev.effective_fps !== undefined) merged.effective_fps = prev.effective_fps;
  if (prev.consumer_count !== undefined && next.consumer_count === undefined) {
    merged.consumer_count = prev.consumer_count;
  }
  return merged;
}

// computeEffectiveFps derives a measured publish rate from consecutive status
// snapshots: Δ real_frames / Δ ts. undefined on the first sample, counter
// reset, non-monotonic ts, or a sub-100ms window (noise floor).
function computeEffectiveFps(prev: StatusParams | undefined, next: StatusParams): number | undefined {
  const prevTs = prev?.ts_ms;
  const prevFrames = prev?.broadcast.real_frames;
  if (prevTs === undefined || prevFrames === undefined) return undefined;
  const dt = next.ts_ms - prevTs;
  const df = next.broadcast.real_frames - prevFrames;
  if (dt < 100 || df < 0) return undefined;
  return Math.round((df / (dt / 1000)) * 10) / 10;
}
