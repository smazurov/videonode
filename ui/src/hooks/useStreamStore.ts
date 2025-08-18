import { create } from 'zustand';
import { subscribeWithSelector } from 'zustand/middleware';
import { createStreamDataSlice, StreamDataSlice } from './slices/streamDataSlice';
import { createUIStateSlice, UIStateSlice } from './slices/uiStateSlice';
import { createAPISlice, APISlice } from './slices/apiSlice';
import { createDeduplicationSlice, DeduplicationSlice } from './slices/deduplicationSlice';

// Export the combined store interface
export interface StreamStore extends 
  StreamDataSlice,
  UIStateSlice,
  APISlice,
  DeduplicationSlice {}

// Create the store with all slices
export const useStreamStore = create<StreamStore>()(
  subscribeWithSelector(
    (...args) => ({
      ...createStreamDataSlice(...args),
      ...createUIStateSlice(...args),
      ...createAPISlice(...args),
      ...createDeduplicationSlice(...args),
    })
  )
);