import { create } from 'zustand';

// Stub source store — replaced by U2's full slice-based implementation.
// Shape mirrors the canonical Source type from the plan; SSE handlers in
// useSSEManager populate `sourcesById` so downstream consumers can read
// once U2 lands without changing the SSE wiring.
export interface SourceData {
  id: string;
  device?: string;
  test_mode?: boolean;
  created_at?: string;
  updated_at?: string;
}

interface SourceStore {
  sourceIds: string[];
  sourcesById: Record<string, SourceData>;

  upsertSource: (source: SourceData) => void;
  removeSource: (sourceId: string) => void;
}

function sortIds(ids: string[]): string[] {
  return [...ids].sort((a, b) => a.localeCompare(b));
}

export const useSourceStore = create<SourceStore>((set) => ({
  sourceIds: [],
  sourcesById: {},

  upsertSource: (source) => {
    set((state) => {
      const existed = !!state.sourcesById[source.id];
      const sourcesById = { ...state.sourcesById, [source.id]: source };
      const sourceIds = existed
        ? sortIds(state.sourceIds)
        : sortIds([...state.sourceIds, source.id]);
      return { sourceIds, sourcesById };
    });
  },

  removeSource: (sourceId) => {
    set((state) => {
      // eslint-disable-next-line @typescript-eslint/no-unused-vars, sonarjs/no-unused-vars
      const { [sourceId]: _, ...rest } = state.sourcesById;
      return {
        sourceIds: state.sourceIds.filter((id) => id !== sourceId),
        sourcesById: rest,
      };
    });
  },
}));
