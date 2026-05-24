import { create } from 'zustand';
import { subscribeWithSelector } from 'zustand/middleware';
import type { ComposerData, ComposerListData } from '../lib/composer-types';

// Backend endpoints (B6) are not regenerated into api.generated.ts in this
// worktree yet. Until that lands, we hit /api/composers directly via fetch
// and accept the typed shape from composer-types.ts.
import { API_BASE_URL } from '../lib/api';
import { getAuthCredentials } from '../lib/auth';

function authHeaders(): Record<string, string> {
  const credentials = getAuthCredentials();
  const headers: Record<string, string> = { 'Content-Type': 'application/json' };
  if (credentials) headers['Authorization'] = `Basic ${credentials}`;
  return headers;
}

async function fetchComposersJSON(): Promise<ComposerListData> {
  const res = await fetch(`${API_BASE_URL}/api/composers`, {
    headers: authHeaders(),
  });
  if (!res.ok) {
    throw new Error(`Failed to fetch composers (${res.status})`);
  }
  return (await res.json()) as ComposerListData;
}

async function deleteComposerHTTP(id: string): Promise<void> {
  const res = await fetch(`${API_BASE_URL}/api/composers/${encodeURIComponent(id)}`, {
    method: 'DELETE',
    headers: authHeaders(),
  });
  if (!res.ok) {
    let detail = `Failed to delete composer (${res.status})`;
    try {
      const body = (await res.json()) as { detail?: string };
      if (body?.detail) detail = body.detail;
    } catch {
      // body wasn't JSON — keep generic message
    }
    throw new Error(detail);
  }
}

function sortComposerIds(ids: string[]): string[] {
  return [...ids].sort((a, b) => a.localeCompare(b));
}

export interface ComposerStoreState {
  composerIds: string[];
  composersById: Record<string, ComposerData>;
  loading: boolean;
  error: string | null;

  setComposers: (data: ComposerListData) => void;
  addComposer: (composer: ComposerData) => void;
  removeComposer: (composerId: string) => void;
  getComposerById: (composerId: string) => ComposerData | undefined;

  fetchComposers: () => Promise<void>;
  deleteComposer: (composerId: string) => Promise<void>;
}

export const useComposerStore = create<ComposerStoreState>()(
  subscribeWithSelector((set, get) => ({
    composerIds: [],
    composersById: {},
    loading: false,
    error: null,

    setComposers: (data) => {
      const byId: Record<string, ComposerData> = {};
      for (const c of data.composers ?? []) {
        byId[c.composer_id] = c;
      }
      set({
        composersById: byId,
        composerIds: sortComposerIds(Object.keys(byId)),
      });
    },

    addComposer: (composer) => {
      set((state) => {
        const existed = !!state.composersById[composer.composer_id];
        const composersById = {
          ...state.composersById,
          [composer.composer_id]: composer,
        };
        const composerIds = existed
          ? sortComposerIds(state.composerIds)
          : sortComposerIds([...state.composerIds, composer.composer_id]);
        return { composerIds, composersById };
      });
    },

    removeComposer: (composerId) => {
      set((state) => {
        const rest: Record<string, ComposerData> = {};
        for (const [id, composer] of Object.entries(state.composersById)) {
          if (id !== composerId) rest[id] = composer;
        }
        return {
          composerIds: state.composerIds.filter((id) => id !== composerId),
          composersById: rest,
        };
      });
    },

    getComposerById: (composerId) => get().composersById[composerId],

    fetchComposers: async () => {
      const hasExisting = get().composerIds.length > 0;
      if (!hasExisting) set({ loading: true });
      try {
        const data = await fetchComposersJSON();
        get().setComposers(data);
        set({ error: null });
      } catch (error) {
        set({ error: error instanceof Error ? error.message : 'Failed to fetch composers' });
      } finally {
        if (!hasExisting) set({ loading: false });
      }
    },

    deleteComposer: async (composerId) => {
      try {
        await deleteComposerHTTP(composerId);
        get().removeComposer(composerId);
        set({ error: null });
      } catch (error) {
        const msg = error instanceof Error ? error.message : 'Failed to delete composer';
        set({ error: msg });
        throw error;
      }
    },
  })),
);
