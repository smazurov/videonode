import { create } from 'zustand';
import { subscribeWithSelector } from 'zustand/middleware';
import { createStreamDataSlice, StreamDataSlice } from './slices/streamDataSlice';
import { createUIStateSlice, UIStateSlice } from './slices/uiStateSlice';
import { createAPISlice, APISlice } from './slices/apiSlice';

export interface StreamStore extends
  StreamDataSlice,
  UIStateSlice,
  APISlice {}

export const useStreamStore = create<StreamStore>()(
  subscribeWithSelector(
    (...args) => ({
      ...createStreamDataSlice(...args),
      ...createUIStateSlice(...args),
      ...createAPISlice(...args),
    })
  )
);
