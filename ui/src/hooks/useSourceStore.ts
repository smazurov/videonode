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

export type { Source } from './slices/types';
export type { SourceData, SourceRequestData } from './slices/types';
export type { Source as SourceEntry } from './slices/types';
export interface SourceConsumerRef {
  kind: 'composer' | 'stream';
  id: string;
}

export interface SourceStore
  extends SourceDataSlice,
    SourceUIStateSlice,
    SourceAPISlice {}

export const useSourceStore = create<SourceStore>()(
  subscribeWithSelector((set, get, store) => {
    const data = createSourceDataSlice(set, get, store);
    const ui = createSourceUIStateSlice(set, get, store);
    const api = createSourceAPISlice(set, get, store);
    return {
      ...data,
      ...ui,
      ...api,
    };
  }),
);
