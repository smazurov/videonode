import { StateCreator } from 'zustand';

export interface SourceApiSlice {
  loading: boolean;
  error: string | null;
  setLoading: (loading: boolean) => void;
  setError: (error: string | null) => void;
  fetchSources: () => Promise<void>;
}

// Placeholder until B5 lands /api/sources. Marks loading complete so detail
// pages don't spin forever on a missing endpoint; SSE source-status events
// populate the data slice as they arrive.
export const createSourceApiSlice: StateCreator<
  SourceApiSlice,
  [],
  [],
  SourceApiSlice
> = (set) => ({
  loading: false,
  error: null,
  setLoading: (loading) => set({ loading }),
  setError: (error) => set({ error }),
  fetchSources: async () => {
    set({ loading: false, error: null });
  },
});
