import { create } from 'zustand';
import { subscribeWithSelector } from 'zustand/middleware';
import { createSourceDataSlice, type SourceDataSlice } from './slices/sourceDataSlice';
import { createSourceApiSlice, type SourceApiSlice } from './slices/sourceApiSlice';

// Stub for U2 — the real store will own /api/sources CRUD once B5 lands.
// Today we maintain a local catalog seeded by SSE source-status events and
// stream upstream refs, so the /sources surface can render even before the
// backend endpoints exist.

export type {
  SourceStatus,
  SourceConsumerRef,
  SourceEntry,
} from './slices/sourceDataSlice';

export interface SourceStore extends SourceDataSlice, SourceApiSlice {}

export const useSourceStore = create<SourceStore>()(
  subscribeWithSelector((...args) => ({
    ...createSourceDataSlice(...args),
    ...createSourceApiSlice(...args),
  })),
);
