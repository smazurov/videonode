import { create } from 'zustand';
import { subscribeWithSelector } from 'zustand/middleware';

// Stub store: U2 will replace this with the canonical slice-based store.
// Kept type-compatible with the planned shape so consumers (U10) can move
// without changes once U2 lands.

export type ComposerEffect = {
  type: string;
  corners?: [
    [number, number],
    [number, number],
    [number, number],
    [number, number],
  ];
};

export type ComposerInput = {
  ref: string;
  effect?: ComposerEffect | null;
};

export type ComposerCanvas = {
  w: number;
  h: number;
};

export type Composer = {
  id: string;
  canvas: ComposerCanvas;
  inputs: ComposerInput[];
};

export type AvailableSource = {
  id: string;
  label?: string;
};

export interface ComposerStore {
  composersById: Record<string, Composer>;
  availableSources: AvailableSource[];

  setComposer: (composer: Composer) => void;
  setAvailableSources: (sources: AvailableSource[]) => void;
  upsertInput: (composerId: string, input: ComposerInput) => void;
  removeInput: (composerId: string, ref: string) => void;
  setInputEffect: (composerId: string, ref: string, effect: ComposerEffect | null) => void;
}

export const useComposerStore = create<ComposerStore>()(
  subscribeWithSelector((set) => ({
    composersById: {},
    availableSources: [],

    setComposer: (composer) => set((state) => ({
      composersById: { ...state.composersById, [composer.id]: composer },
    })),
    setAvailableSources: (sources) => set({ availableSources: sources }),

    upsertInput: (composerId, input) => set((state) => {
      const existing = state.composersById[composerId];
      if (!existing) return state;
      const others = existing.inputs.filter((i) => i.ref !== input.ref);
      return {
        composersById: {
          ...state.composersById,
          [composerId]: { ...existing, inputs: [...others, input] },
        },
      };
    }),

    removeInput: (composerId, ref) => set((state) => {
      const existing = state.composersById[composerId];
      if (!existing) return state;
      return {
        composersById: {
          ...state.composersById,
          [composerId]: {
            ...existing,
            inputs: existing.inputs.filter((i) => i.ref !== ref),
          },
        },
      };
    }),

    setInputEffect: (composerId, ref, effect) => set((state) => {
      const existing = state.composersById[composerId];
      if (!existing) return state;
      return {
        composersById: {
          ...state.composersById,
          [composerId]: {
            ...existing,
            inputs: existing.inputs.map((i) =>
              i.ref === ref ? { ...i, effect } : i
            ),
          },
        },
      };
    }),
  }))
);
