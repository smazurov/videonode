import { StateCreator } from 'zustand';
import { StreamStore } from '../useStreamStore';

export type ConnectionStatus = 'online' | 'offline' | 'reconnecting';

export interface UIStateSlice {
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

export const createUIStateSlice: StateCreator<
  StreamStore,
  [],
  [],
  UIStateSlice
> = (set) => ({
  ...initialUIState,

  setLoading: (loading) => set({ loading }),

  setError: (error) => set({ error, loading: false }),

  setConnectionStatus: (connectionStatus) => set({ connectionStatus }),

  reset: () => set(() => ({
    ...initialUIState,
    streamIds: [],
    streamsById: {},
    recentOperations: new Map(),
  })),
});