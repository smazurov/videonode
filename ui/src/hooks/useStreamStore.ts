import { create } from 'zustand';
import { subscribeWithSelector } from 'zustand/middleware';

import {
  createStreamDataSlice,
  StreamDataSlice,
} from './slices/streams/streamDataSlice';
import {
  createStreamUIStateSlice,
  StreamUIStateSlice,
} from './slices/streams/streamUIStateSlice';
import {
  createStreamAPISlice,
  StreamAPISlice,
} from './slices/streams/streamApiSlice';
import { createPipelineSlice, PipelineSlice } from './slices/pipelineSlice';

export interface StreamStore
  extends StreamDataSlice,
    StreamUIStateSlice,
    StreamAPISlice,
    PipelineSlice {}

export const useStreamStore = create<StreamStore>()(
  subscribeWithSelector((...args) => ({
    ...createStreamDataSlice(...args),
    ...createStreamUIStateSlice(...args),
    ...createStreamAPISlice(...args),
    ...createPipelineSlice(...args),
  })),
);
