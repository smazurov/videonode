import { create } from 'zustand';

// Stub composer store — replaced by U2's full slice-based implementation.
// Shape mirrors the canonical Composer type from the plan; SSE handlers in
// useSSEManager populate `composersById` so downstream consumers can read
// once U2 lands without changing the SSE wiring.
export interface ComposerCanvasDims {
  w: number;
  h: number;
}

export interface ComposerInputData {
  ref: string;
  effect?: unknown;
}

export interface ComposerLayoutSlot {
  input: string;
  x: number;
  y: number;
  w: number;
  h: number;
}

export interface ComposerData {
  id: string;
  canvas: ComposerCanvasDims;
  inputs: ComposerInputData[];
  layout: ComposerLayoutSlot[];
  created_at?: string;
  updated_at?: string;
}

interface ComposerStore {
  composerIds: string[];
  composersById: Record<string, ComposerData>;

  upsertComposer: (composer: ComposerData) => void;
  removeComposer: (composerId: string) => void;
  updateLayout: (composerId: string, layout: ComposerLayoutSlot[]) => void;
}

function sortIds(ids: string[]): string[] {
  return [...ids].sort((a, b) => a.localeCompare(b));
}

export const useComposerStore = create<ComposerStore>((set) => ({
  composerIds: [],
  composersById: {},

  upsertComposer: (composer) => {
    set((state) => {
      const existed = !!state.composersById[composer.id];
      const composersById = { ...state.composersById, [composer.id]: composer };
      const composerIds = existed
        ? sortIds(state.composerIds)
        : sortIds([...state.composerIds, composer.id]);
      return { composerIds, composersById };
    });
  },

  removeComposer: (composerId) => {
    set((state) => {
      // eslint-disable-next-line @typescript-eslint/no-unused-vars, sonarjs/no-unused-vars
      const { [composerId]: _, ...rest } = state.composersById;
      return {
        composerIds: state.composerIds.filter((id) => id !== composerId),
        composersById: rest,
      };
    });
  },

  updateLayout: (composerId, layout) => {
    set((state) => {
      const existing = state.composersById[composerId];
      if (!existing) return state;
      return {
        composersById: {
          ...state.composersById,
          [composerId]: { ...existing, layout },
        },
      };
    });
  },
}));
