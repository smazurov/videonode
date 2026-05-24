import { StateCreator } from 'zustand';

import type { Composer } from '../types';
import { ComposerStore } from '../../useComposerStore';
import { assertNever, type EntityAction } from '../../entityTypes';

export interface ComposerDataSlice {
  composerIds: string[];
  composersById: Record<string, Composer>;
  // Live runtime slots populated by EntityEvent action=status|metrics|consumers.
  statusById: Record<string, unknown>;
  metricsById: Record<string, unknown>;
  consumersById: Record<string, unknown>;

  setComposers: (composers: Composer[] | null | undefined) => void;
  addComposer: (composer: Composer) => void;
  removeComposer: (composerId: string) => void;
  getComposerById: (composerId: string) => Composer | undefined;
  applyEntityEvent: (
    action: EntityAction,
    id: string,
    payload: unknown,
  ) => void;
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
  statusById: {},
  metricsById: {},
  consumersById: {},

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

  applyEntityEvent: (action, id, payload) => {
    const { addComposer, removeComposer } = get();
    switch (action) {
      case 'created':
      case 'updated':
        if (payload) addComposer(payload as Composer);
        return;
      case 'deleted':
        removeComposer(id);
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
