import { StateCreator } from 'zustand';

import type { Source } from '../types';
import { SourceStore } from '../../useSourceStore';

export interface SourceDataSlice {
  sourceIds: string[];
  sourcesById: Record<string, Source>;

  setSources: (sources: Source[] | null | undefined) => void;
  addSource: (source: Source) => void;
  removeSource: (sourceId: string) => void;
  getSourceById: (sourceId: string) => Source | undefined;
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
});
