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
import type { Source } from './slices/types';

// Re-exports for backwards compat with code that imported these names from
// the store module (earlier U3/U7 store stubs).
export type { Source } from './slices/types';
export type SourceData = Source;
export type SourceRequestData = { id: string; device?: string; test_mode?: boolean };
export interface SourceConsumerRef {
  kind: 'composer' | 'stream';
  id: string;
}

export interface SourceStore
  extends SourceDataSlice,
    SourceUIStateSlice,
    SourceAPISlice {
  // Compatibility surfaces — no-op stubs so older callers compile.
  upsertSource: (source: Source) => void;
  getAllSources: () => Source[];
  setConsumers: (sourceId: string, consumers: SourceConsumerRef[]) => void;
  applyStatusEvent: (event: unknown) => void;
  getReferencesTo: (sourceId: string) => { composers: string[]; streams: string[] };
}

export const useSourceStore = create<SourceStore>()(
  subscribeWithSelector((set, get, store) => {
    const data = createSourceDataSlice(set, get, store);
    const ui = createSourceUIStateSlice(set, get, store);
    const api = createSourceAPISlice(set, get, store);
    return {
      ...data,
      ...ui,
      ...api,
      upsertSource: data.addSource,
      getAllSources: () => Object.values(get().sourcesById),
      setConsumers: () => undefined,
      applyStatusEvent: () => undefined,
      getReferencesTo: () => ({ composers: [], streams: [] }),
    };
  }),
);
