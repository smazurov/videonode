import { StateCreator } from 'zustand';

import { API_BASE_URL } from '../../../lib/api';
import { getAuthCredentials } from '../../../lib/auth';
import type {
  Composer,
  ComposerRequest,
  LayoutSlot,
  Effect,
} from '../types';

interface ComposerList {
  composers?: Composer[] | null;
  count?: number;
}
import { ComposerStore } from '../../useComposerStore';

const COMPOSERS_PATH = '/api/composers';

async function requestJSON<T>(
  method: string,
  path: string,
  body?: unknown,
  fallbackMsg = 'Request failed',
): Promise<T> {
  const credentials = getAuthCredentials();
  const headers: HeadersInit = { 'Content-Type': 'application/json' };
  if (credentials) headers['Authorization'] = `Basic ${credentials}`;
  const response = await fetch(`${API_BASE_URL}${path}`, {
    method,
    headers,
    body: body === undefined ? null : JSON.stringify(body),
  });
  if (!response.ok) {
    let detail = fallbackMsg;
    try {
      const data = (await response.json()) as { detail?: string };
      if (data.detail) detail = data.detail;
    } catch {
      // ignore body parse failures
    }
    throw new Error(detail);
  }
  if (response.status === 204) return undefined as T;
  return (await response.json()) as T;
}

export interface ComposerAPISlice {
  fetchComposers: () => Promise<void>;
  listComposers: () => Promise<Composer[]>;
  getComposer: (composerId: string) => Promise<Composer>;
  createComposer: (request: ComposerRequest) => Promise<Composer>;
  updateComposer: (
    composerId: string,
    data: Partial<ComposerRequest>,
  ) => Promise<Composer>;
  deleteComposer: (composerId: string) => Promise<void>;
  updateComposerLayout: (
    composerId: string,
    layout: LayoutSlot[],
  ) => Promise<Composer>;
  updateComposerInputEffect: (
    composerId: string,
    inputRef: string,
    effect: Effect | null,
  ) => Promise<Composer>;
}

export const createComposerAPISlice: StateCreator<
  ComposerStore,
  [],
  [],
  ComposerAPISlice
> = (_set, get) => ({
  fetchComposers: async () => {
    const { setLoading, setError, setComposers, composerIds } = get();

    const hasExisting = composerIds.length > 0;
    if (!hasExisting) setLoading(true);

    try {
      const data = await requestJSON<ComposerList>(
        'GET',
        COMPOSERS_PATH,
        undefined,
        'Failed to fetch composers',
      );
      setComposers(data.composers ?? null);
      setError(null);
    } catch (error) {
      setError(error instanceof Error ? error.message : 'Failed to fetch composers');
    } finally {
      if (!hasExisting) setLoading(false);
    }
  },

  listComposers: async () => {
    const data = await requestJSON<ComposerList>(
      'GET',
      COMPOSERS_PATH,
      undefined,
      'Failed to list composers',
    );
    return data.composers ?? [];
  },

  getComposer: async (composerId) => {
    return requestJSON<Composer>(
      'GET',
      `${COMPOSERS_PATH}/${encodeURIComponent(composerId)}`,
      undefined,
      'Failed to get composer',
    );
  },

  createComposer: async (request) => {
    const { setError, addComposer } = get();
    try {
      const data = await requestJSON<Composer>(
        'POST',
        COMPOSERS_PATH,
        request,
        'Failed to create composer',
      );
      addComposer(data);
      setError(null);
      return data;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Failed to create composer';
      setError(message);
      throw error;
    }
  },

  updateComposer: async (composerId, data) => {
    const { setError, addComposer } = get();
    try {
      const composer = await requestJSON<Composer>(
        'PATCH',
        `${COMPOSERS_PATH}/${encodeURIComponent(composerId)}`,
        data,
        'Failed to update composer',
      );
      addComposer(composer);
      setError(null);
      return composer;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Failed to update composer';
      setError(message);
      throw error;
    }
  },

  deleteComposer: async (composerId) => {
    const { setError, removeComposer } = get();
    try {
      await requestJSON<void>(
        'DELETE',
        `${COMPOSERS_PATH}/${encodeURIComponent(composerId)}`,
        undefined,
        'Failed to delete composer',
      );
      removeComposer(composerId);
      setError(null);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Failed to delete composer';
      setError(message);
      throw error;
    }
  },

  updateComposerLayout: async (composerId, layout) => {
    const { setError, addComposer } = get();
    try {
      const composer = await requestJSON<Composer>(
        'PATCH',
        `${COMPOSERS_PATH}/${encodeURIComponent(composerId)}/layout`,
        { layout },
        'Failed to update composer layout',
      );
      addComposer(composer);
      setError(null);
      return composer;
    } catch (error) {
      const message =
        error instanceof Error ? error.message : 'Failed to update composer layout';
      setError(message);
      throw error;
    }
  },

  updateComposerInputEffect: async (composerId, inputRef, effect) => {
    const { setError, addComposer } = get();
    try {
      const composer = await requestJSON<Composer>(
        'PATCH',
        `${COMPOSERS_PATH}/${encodeURIComponent(composerId)}/inputs/${encodeURIComponent(inputRef)}/effect`,
        { effect },
        'Failed to update input effect',
      );
      addComposer(composer);
      setError(null);
      return composer;
    } catch (error) {
      const message =
        error instanceof Error ? error.message : 'Failed to update input effect';
      setError(message);
      throw error;
    }
  },
});
