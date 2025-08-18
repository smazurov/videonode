import { StateCreator } from 'zustand';
import { StreamStore } from '../useStreamStore';

export interface UIStateSlice {
  loading: boolean;
  error: string | null;
  lastUpdated: Date | null;
  
  setLoading: (loading: boolean) => void;
  setError: (error: string | null) => void;
  reset: () => void;
}

const initialUIState = {
  loading: false,
  error: null,
  lastUpdated: null,
};

export const createUIStateSlice: StateCreator<
  StreamStore,
  [],
  [],
  UIStateSlice
> = (set) => ({
  ...initialUIState,
  
  setLoading: (loading) => set({ loading }),
  
  setError: (error) => set({ error, loading: false }),
  
  reset: () => set(() => ({
    ...initialUIState,
    streams: new Map(),
    recentOperations: new Map(),
  })),
});