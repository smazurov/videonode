import { StateCreator } from 'zustand';

import { SourceStore } from '../../useSourceStore';

export type ConnectionStatus = 'online' | 'offline' | 'reconnecting';

export interface SourceUIStateSlice {
  loading: boolean;
  error: string | null;
  lastUpdated: Date | null;
  connectionStatus: ConnectionStatus;

  setLoading: (loading: boolean) => void;
  setError: (error: string | null) => void;
  setConnectionStatus: (status: ConnectionStatus) => void;
  reset: () => void;
}

const initialUIState = {
  loading: false,
  error: null,
  lastUpdated: null,
  connectionStatus: 'offline' as ConnectionStatus,
};

export const createSourceUIStateSlice: StateCreator<
  SourceStore,
  [],
  [],
  SourceUIStateSlice
> = (set) => ({
  ...initialUIState,

  setLoading: (loading) => set({ loading }),

  setError: (error) => set({ error, loading: false }),

  setConnectionStatus: (connectionStatus) => set({ connectionStatus }),

  reset: () =>
    set(() => ({
      ...initialUIState,
      sourceIds: [],
      sourcesById: {},
    })),
});
