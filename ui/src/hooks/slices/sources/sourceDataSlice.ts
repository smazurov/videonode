import { StateCreator } from 'zustand';

import type { Source } from '../types';
import { SourceStore } from '../../useSourceStore';
import { assertNever, type EntityAction } from '../../entityTypes';

export interface SourceDataSlice {
  sourceIds: string[];
  sourcesById: Record<string, Source>;
  // Live runtime slots populated by EntityEvent action=status|metrics|consumers.
  // Components select narrowly: useSourceStore(s => s.statusById[id]).
  statusById: Record<string, unknown>;
  metricsById: Record<string, unknown>;
  consumersById: Record<string, unknown>;

  setSources: (sources: Source[] | null | undefined) => void;
  addSource: (source: Source) => void;
  removeSource: (sourceId: string) => void;
  getSourceById: (sourceId: string) => Source | undefined;
  applyEntityEvent: (
    action: EntityAction,
    id: string,
    payload: unknown,
  ) => void;
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
  metricsById: {},
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
      // eslint-disable-next-line @typescript-eslint/no-unused-vars, sonarjs/no-unused-vars
      const { [sourceId]: _, ...rest } = state.sourcesById;
      return {
        sourceIds: state.sourceIds.filter((id) => id !== sourceId),
        sourcesById: rest,
        lastUpdated: new Date(),
      };
    });
  },

  getSourceById: (sourceId) => get().sourcesById[sourceId],

  applyEntityEvent: (action, id, payload) => {
    const { addSource, removeSource } = get();
    switch (action) {
      case 'created':
      case 'updated':
        if (payload) addSource(payload as Source);
        return;
      case 'deleted':
        removeSource(id);
        return;
      case 'status': {
        const snap = payload as Partial<{
          health: string;
          ts_ms: number;
          started_at_us: number;
          broadcast: { real_frames?: number };
        }> | null | undefined;
        set((state) => {
          const next = { ...state.statusById, [id]: payload };
          const src = state.sourcesById[id];
          if (!src || !snap) return { statusById: next };
          const lastAt = snap.ts_ms ? new Date(snap.ts_ms).toISOString() : new Date().toISOString();
          const merged: Source = { ...src, last_status_at: lastAt };
          if (payload) merged.latest_status = payload as NonNullable<Source['latest_status']>;
          // Lift the per-second health token onto the top-level liveness
          // field so list rows (which never read latest_status) update live.
          if (typeof snap.health === 'string') {
            merged.liveness = snap.health as NonNullable<Source['liveness']>;
          }
          if (typeof snap.started_at_us === 'number' && snap.started_at_us > 0) {
            merged.started_at_us = snap.started_at_us;
          } else {
            delete merged.started_at_us;
          }
          const fps = computeEffectiveFps(state.statusById[id], snap);
          if (fps !== undefined) merged.effective_fps = fps;
          else delete merged.effective_fps;
          return {
            statusById: next,
            sourcesById: { ...state.sourcesById, [id]: merged },
          };
        });
        return;
      }
      case 'metrics':
        set((state) => ({
          metricsById: { ...state.metricsById, [id]: payload },
        }));
        return;
      case 'consumers': {
        const snap = payload as Partial<{ count: number }> | null | undefined;
        set((state) => {
          const next = { ...state.consumersById, [id]: payload };
          const src = state.sourcesById[id];
          if (!src || !snap || typeof snap.count !== 'number') {
            return { consumersById: next };
          }
          return {
            consumersById: next,
            sourcesById: {
              ...state.sourcesById,
              [id]: { ...src, consumer_count: snap.count },
            },
          };
        });
        return;
      }
      default:
        assertNever(action);
    }
  },
});

// mergeRuntime overlays a fresh API payload onto an existing in-store
// entry while preserving the runtime-augmented fields the UI maintains
// itself (status pill, latest status snapshot, last update timestamp,
// process start time, effective fps, consumer count). Without this,
// every dependency-driven `source.updated` republish would wipe the
// UI's live state until the next status SSE arrived.
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

// computeEffectiveFps derives a measured publish rate from consecutive
// status snapshots: Δ real_frames / Δ ts. Returns undefined for the
// first sample, when the counter resets, when ts is non-monotonic, or
// when the window is shorter than 100ms (noise floor).
function computeEffectiveFps(prev: unknown, next: {
  ts_ms?: number;
  broadcast?: { real_frames?: number };
}): number | undefined {
  const p = prev as
    | { ts_ms?: number; broadcast?: { real_frames?: number } }
    | undefined;
  const prevTs = p?.ts_ms;
  const prevFrames = p?.broadcast?.real_frames;
  const nextTs = next.ts_ms;
  const nextFrames = next.broadcast?.real_frames;
  if (
    typeof prevTs !== 'number' ||
    typeof nextTs !== 'number' ||
    typeof prevFrames !== 'number' ||
    typeof nextFrames !== 'number'
  ) {
    return undefined;
  }
  const dt = nextTs - prevTs;
  const df = nextFrames - prevFrames;
  if (dt < 100 || df < 0) return undefined;
  return Math.round((df / (dt / 1000)) * 10) / 10;
}

