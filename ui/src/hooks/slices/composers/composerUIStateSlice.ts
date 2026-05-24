import { StateCreator } from 'zustand';

import { ComposerStore } from '../../useComposerStore';

export type ConnectionStatus = 'online' | 'offline' | 'reconnecting';

export interface ComposerUIStateSlice {
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

export const createComposerUIStateSlice: StateCreator<
  ComposerStore,
  [],
  [],
  ComposerUIStateSlice
> = (set) => ({
  ...initialUIState,

  setLoading: (loading) => set({ loading }),

  setError: (error) => set({ error, loading: false }),

  setConnectionStatus: (connectionStatus) => set({ connectionStatus }),

  reset: () =>
    set(() => ({
      ...initialUIState,
      composerIds: [],
      composersById: {},
    })),
});
