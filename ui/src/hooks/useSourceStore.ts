import { create } from 'zustand';

// Stub source store — replaced by U2. Shape kept compatible with the
// canonical Source type from the plan (id, device, test_mode, timestamps).
// API calls are local-state-only stubs until U2 + B5 land; UI plumbing
// (forms, routes, validation) can be exercised against this without
// blocking on the backend.

export interface SourceData {
  id: string;
  device: string;
  test_mode: boolean;
  created_at: string;
  updated_at: string;
}

export interface SourceRequestData {
  id: string;
  device?: string;
  test_mode?: boolean;
}

export interface SourceReferences {
  composers: string[];
  streams: string[];
}

interface SourceStore {
  sourcesById: Record<string, SourceData>;
  loading: boolean;
  error: string | null;
  lastUpdated: Date | null;

  getAllSources: () => SourceData[];
  getSourceById: (id: string) => SourceData | undefined;
  getReferencesTo: (id: string) => SourceReferences;

  fetchSources: () => Promise<void>;
  createSource: (req: SourceRequestData) => Promise<SourceData>;
  updateSource: (id: string, req: Partial<SourceRequestData>) => Promise<SourceData>;
  deleteSource: (id: string) => Promise<void>;
}

function nowIso(): string {
  return new Date().toISOString();
}

export const useSourceStore = create<SourceStore>((set, get) => ({
  sourcesById: {},
  loading: false,
  error: null,
  lastUpdated: null,

  getAllSources: () => Object.values(get().sourcesById),

  getSourceById: (id) => get().sourcesById[id],

  // Stubbed — U2/B5 will surface real composer/stream cross-refs.
  getReferencesTo: () => ({ composers: [], streams: [] }),

  fetchSources: async () => {
    set({ lastUpdated: new Date() });
  },

  createSource: async (req) => {
    const source: SourceData = {
      id: req.id,
      device: req.device ?? '',
      test_mode: req.test_mode ?? false,
      created_at: nowIso(),
      updated_at: nowIso(),
    };
    set((state) => ({
      sourcesById: { ...state.sourcesById, [source.id]: source },
      lastUpdated: new Date(),
    }));
    return source;
  },

  updateSource: async (id, req) => {
    const existing = get().sourcesById[id];
    if (!existing) throw new Error(`Source ${id} not found`);
    const next: SourceData = {
      ...existing,
      ...(req.device !== undefined ? { device: req.device } : {}),
      ...(req.test_mode !== undefined ? { test_mode: req.test_mode } : {}),
      updated_at: nowIso(),
    };
    set((state) => ({
      sourcesById: { ...state.sourcesById, [id]: next },
      lastUpdated: new Date(),
    }));
    return next;
  },

  deleteSource: async (id) => {
    set((state) => {
      // eslint-disable-next-line @typescript-eslint/no-unused-vars, sonarjs/no-unused-vars
      const { [id]: _, ...rest } = state.sourcesById;
      return { sourcesById: rest, lastUpdated: new Date() };
    });
  },
}));
