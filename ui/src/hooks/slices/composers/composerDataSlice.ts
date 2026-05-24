import { StateCreator } from 'zustand';

import type { Composer } from '../types';
import { ComposerStore } from '../../useComposerStore';

export interface ComposerDataSlice {
  composerIds: string[];
  composersById: Record<string, Composer>;

  setComposers: (composers: Composer[] | null | undefined) => void;
  addComposer: (composer: Composer) => void;
  removeComposer: (composerId: string) => void;
  getComposerById: (composerId: string) => Composer | undefined;
}

function sortIds(ids: string[]): string[] {
  return [...ids].sort((a, b) => a.localeCompare(b));
}

export const createComposerDataSlice: StateCreator<
  ComposerStore,
  [],
  [],
  ComposerDataSlice
> = (set, get) => ({
  composerIds: [],
  composersById: {},

  setComposers: (composers) => {
    const byId: Record<string, Composer> = {};
    for (const composer of composers ?? []) {
      byId[composer.id] = composer;
    }
    const ids = sortIds(Object.keys(byId));
    set(() => ({
      composerIds: ids,
      composersById: byId,
      lastUpdated: new Date(),
    }));
  },

  addComposer: (composer) => {
    set((state) => {
      const existed = !!state.composersById[composer.id];
      const composersById = { ...state.composersById, [composer.id]: composer };
      const nextIds = existed
        ? sortIds(state.composerIds)
        : sortIds([...state.composerIds, composer.id]);
      return {
        composerIds: nextIds,
        composersById,
        lastUpdated: new Date(),
      };
    });
  },

  removeComposer: (composerId) => {
    set((state) => {
      // eslint-disable-next-line @typescript-eslint/no-unused-vars, sonarjs/no-unused-vars
      const { [composerId]: _, ...rest } = state.composersById;
      return {
        composerIds: state.composerIds.filter((id) => id !== composerId),
        composersById: rest,
        lastUpdated: new Date(),
      };
    });
  },

  getComposerById: (composerId) => get().composersById[composerId],
});
