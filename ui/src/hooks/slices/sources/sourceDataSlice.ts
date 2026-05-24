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
      case 'status':
        set((state) => ({
          statusById: { ...state.statusById, [id]: payload },
        }));
        return;
      case 'metrics':
        set((state) => ({
          metricsById: { ...state.metricsById, [id]: payload },
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
