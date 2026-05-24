import { create } from 'zustand';
import { subscribeWithSelector } from 'zustand/middleware';

import {
  createComposerDataSlice,
  ComposerDataSlice,
} from './slices/composers/composerDataSlice';
import {
  createComposerUIStateSlice,
  ComposerUIStateSlice,
} from './slices/composers/composerUIStateSlice';
import {
  createComposerAPISlice,
  ComposerAPISlice,
} from './slices/composers/composerApiSlice';
import type { Composer, ComposerInput, Effect, LayoutSlot } from './slices/types';

// Re-exports for backwards compat with earlier-era code.
export type { Composer, ComposerInput } from './slices/types';
export type ComposerLayoutSlot = LayoutSlot;
export type ComposerEffect = Effect;
export type ComposerData = Composer;

export interface ComposerStore
  extends ComposerDataSlice,
    ComposerUIStateSlice,
    ComposerAPISlice {
  // Compatibility no-ops for older U10/U11 code.
  upsertComposer: (composer: Composer) => void;
  setComposer: (composer: Composer) => void;
  setAvailableSources: (sources: string[]) => void;
  upsertInput: (composerId: string, input: ComposerInput) => void;
  removeInput: (composerId: string, ref: string) => void;
  setInputEffect: (composerId: string, ref: string, effect: Effect | null) => void;
  availableSources: string[];
}

export const useComposerStore = create<ComposerStore>()(
  subscribeWithSelector((set, get, store) => {
    const data = createComposerDataSlice(set, get, store);
    const ui = createComposerUIStateSlice(set, get, store);
    const api = createComposerAPISlice(set, get, store);
    return {
      ...data,
      ...ui,
      ...api,
      upsertComposer: data.addComposer,
      setComposer: data.addComposer,
      setAvailableSources: () => undefined,
      upsertInput: () => undefined,
      removeInput: () => undefined,
      setInputEffect: () => undefined,
      availableSources: [],
    };
  }),
);
