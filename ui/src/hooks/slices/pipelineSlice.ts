import { StateCreator } from 'zustand';
import { api, unwrap } from '../../lib/api';
import { StreamStore } from '../useStreamStore';

export interface PipelineSlice {
  pipelineEnabled: boolean | null;
  pipelineToggling: boolean;
  setPipelineEnabled: (enabled: boolean) => void;
  fetchPipelineState: () => Promise<void>;
  startPipeline: () => Promise<void>;
  stopPipeline: () => Promise<void>;
}

export const createPipelineSlice: StateCreator<
  StreamStore,
  [],
  [],
  PipelineSlice
> = (set, get) => ({
  pipelineEnabled: null,
  pipelineToggling: false,

  setPipelineEnabled: (pipelineEnabled) => set({ pipelineEnabled }),

  fetchPipelineState: async () => {
    try {
      const data = unwrap(await api.GET('/api/pipeline'), 'Failed to fetch pipeline state');
      set({ pipelineEnabled: data.enabled });
    } catch (error) {
      console.error('Failed to fetch pipeline state', error);
    }
  },

  startPipeline: async () => {
    if (get().pipelineToggling) return;
    set({ pipelineToggling: true });
    try {
      const data = unwrap(await api.POST('/api/pipeline/start'), 'Failed to start pipeline');
      set({ pipelineEnabled: data.enabled });
    } finally {
      set({ pipelineToggling: false });
    }
  },

  stopPipeline: async () => {
    if (get().pipelineToggling) return;
    set({ pipelineToggling: true });
    try {
      const data = unwrap(await api.POST('/api/pipeline/stop'), 'Failed to stop pipeline');
      set({ pipelineEnabled: data.enabled });
    } finally {
      set({ pipelineToggling: false });
    }
  },
});
