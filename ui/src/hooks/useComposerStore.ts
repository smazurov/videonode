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

export interface ComposerStore
  extends ComposerDataSlice,
    ComposerUIStateSlice,
    ComposerAPISlice {}

export const useComposerStore = create<ComposerStore>()(
  subscribeWithSelector((...args) => ({
    ...createComposerDataSlice(...args),
    ...createComposerUIStateSlice(...args),
    ...createComposerAPISlice(...args),
  })),
);
