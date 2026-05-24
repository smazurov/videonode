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
    const byId: Record<string, Source> = {};
    for (const source of sources ?? []) {
      byId[source.id] = source;
    }
    const ids = sortIds(Object.keys(byId));
    set(() => ({
      sourceIds: ids,
      sourcesById: byId,
      lastUpdated: new Date(),
    }));
  },

  addSource: (source) => {
    set((state) => {
      const existed = !!state.sourcesById[source.id];
      const sourcesById = { ...state.sourcesById, [source.id]: source };
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
          broadcast: { real_frames?: number };
        }> | null | undefined;
        set((state) => {
          const next = { ...state.statusById, [id]: payload };
          const src = state.sourcesById[id];
          if (!src || !snap) return { statusById: next };
          const status = healthToPill(snap.health);
          const lastAt = snap.ts_ms ? new Date(snap.ts_ms).toISOString() : new Date().toISOString();
          const prevRunning = src.running_since;
          const merged: Source = { ...src, status, last_status_at: lastAt };
          if (payload) merged.latest_status = payload as NonNullable<Source['latest_status']>;
          if (status === 'running') merged.running_since = prevRunning ?? lastAt;
          else delete merged.running_since;
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

// Map the source binary's collapsed health enum onto the small
// StatusPill vocabulary the UI ships. videonode-source emits health
// strings either lower-case ("live") or upper-case ("INITIALIZING"),
// so normalise before matching.
function healthToPill(
  health: string | undefined,
): 'running' | 'idle' | 'error' | 'warm' | 'stopped' {
  switch ((health ?? '').toLowerCase()) {
    case 'live':
    case 'transitioning':
      return 'running';
    case 'placeholder':
    case 'no_signal':
    case 'initializing':
      return 'warm';
    case 'error':
    case 'failed':
      return 'error';
    case '':
      return 'stopped';
    default:
      return 'idle';
  }
}
