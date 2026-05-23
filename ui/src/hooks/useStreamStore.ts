import { create } from 'zustand';
import { subscribeWithSelector } from 'zustand/middleware';
import { createStreamDataSlice, StreamDataSlice } from './slices/streamDataSlice';
import { createUIStateSlice, UIStateSlice } from './slices/uiStateSlice';
import { createAPISlice, APISlice } from './slices/apiSlice';
import { createPipelineSlice, PipelineSlice } from './slices/pipelineSlice';

export interface StreamStore extends
  StreamDataSlice,
  UIStateSlice,
  APISlice,
  PipelineSlice {}

export const useStreamStore = create<StreamStore>()(
  subscribeWithSelector(
    (...args) => ({
      ...createStreamDataSlice(...args),
      ...createUIStateSlice(...args),
      ...createAPISlice(...args),
      ...createPipelineSlice(...args),
    })
  )
);
