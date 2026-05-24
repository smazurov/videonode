import { create } from 'zustand';
import { subscribeWithSelector } from 'zustand/middleware';

import {
  createSourceDataSlice,
  SourceDataSlice,
} from './slices/sources/sourceDataSlice';
import {
  createSourceUIStateSlice,
  SourceUIStateSlice,
} from './slices/sources/sourceUIStateSlice';
import {
  createSourceAPISlice,
  SourceAPISlice,
} from './slices/sources/sourceApiSlice';

export interface SourceStore
  extends SourceDataSlice,
    SourceUIStateSlice,
    SourceAPISlice {}

export const useSourceStore = create<SourceStore>()(
  subscribeWithSelector((...args) => ({
    ...createSourceDataSlice(...args),
    ...createSourceUIStateSlice(...args),
    ...createSourceAPISlice(...args),
  })),
);
