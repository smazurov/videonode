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
  exportComposerToml: (composerId: string) => Promise<string>;
  importComposerToml: (toml: string) => Promise<Composer>;
  importComposerTomlInto: (composerId: string, toml: string) => Promise<Composer>;
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
    const { addComposer } = get();
    const data = await requestJSON<Composer>(
      'POST',
      COMPOSERS_PATH,
      request,
      'Failed to create composer',
    );
    addComposer(data);
    return data;
  },

  updateComposer: async (composerId, data) => {
    const { addComposer } = get();
    const composer = await requestJSON<Composer>(
      'PATCH',
      `${COMPOSERS_PATH}/${encodeURIComponent(composerId)}`,
      data,
      'Failed to update composer',
    );
    addComposer(composer);
    return composer;
  },

  deleteComposer: async (composerId) => {
    const { removeComposer } = get();
    await requestJSON<void>(
      'DELETE',
      `${COMPOSERS_PATH}/${encodeURIComponent(composerId)}`,
      undefined,
      'Failed to delete composer',
    );
    removeComposer(composerId);
  },

  updateComposerLayout: async (composerId, layout) => {
    const { addComposer } = get();
    const composer = await requestJSON<Composer>(
      'PATCH',
      `${COMPOSERS_PATH}/${encodeURIComponent(composerId)}/layout`,
      { layout },
      'Failed to update composer layout',
    );
    addComposer(composer);
    return composer;
  },

  updateComposerInputEffect: async (composerId, inputRef, effect) => {
    const { addComposer } = get();
    const composer = await requestJSON<Composer>(
      'PATCH',
      `${COMPOSERS_PATH}/${encodeURIComponent(composerId)}/inputs/${encodeURIComponent(inputRef)}/effect`,
      { effect },
      'Failed to update input effect',
    );
    addComposer(composer);
    return composer;
  },

  exportComposerToml: async (composerId) => {
    const credentials = getAuthCredentials();
    const headers: HeadersInit = {};
    if (credentials) headers['Authorization'] = `Basic ${credentials}`;
    const response = await fetch(
      `${API_BASE_URL}${COMPOSERS_PATH}/${encodeURIComponent(composerId)}/export`,
      { method: 'GET', headers },
    );
    if (!response.ok) {
      throw new Error(`Failed to export composer (${response.status})`);
    }
    return response.text();
  },

  importComposerToml: async (toml) => {
    const { addComposer } = get();
    const composer = await postToml(`${COMPOSERS_PATH}/import`, toml);
    addComposer(composer);
    return composer;
  },

  importComposerTomlInto: async (composerId, toml) => {
    const { addComposer } = get();
    const composer = await postToml(
      `${COMPOSERS_PATH}/${encodeURIComponent(composerId)}/import`,
      toml,
    );
    addComposer(composer);
    return composer;
  },
});

async function postToml(path: string, toml: string): Promise<Composer> {
  const credentials = getAuthCredentials();
  const headers: HeadersInit = { 'Content-Type': 'application/toml' };
  if (credentials) headers['Authorization'] = `Basic ${credentials}`;
  const response = await fetch(`${API_BASE_URL}${path}`, {
    method: 'POST',
    headers,
    body: toml,
  });
  if (!response.ok) {
    let detail = 'Failed to import composer';
    try {
      const data = (await response.json()) as { detail?: string };
      if (data.detail) detail = data.detail;
    } catch {
      // ignore body parse failures
    }
    throw new Error(detail);
  }
  return (await response.json()) as Composer;
}
