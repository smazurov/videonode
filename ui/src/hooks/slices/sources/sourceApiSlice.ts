import { StateCreator } from 'zustand';

import { API_BASE_URL } from '../../../lib/api';
import { getAuthCredentials } from '../../../lib/auth';
import type { Source, SourceRequestData as SourceRequest } from '../types';

interface SourceList {
  sources?: Source[] | null;
  count?: number;
}
import { SourceStore } from '../../useSourceStore';

const SOURCES_PATH = '/api/sources';

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

export interface SourceAPISlice {
  fetchSources: () => Promise<void>;
  listSources: () => Promise<Source[]>;
  getSource: (sourceId: string) => Promise<Source>;
  createSource: (request: SourceRequest) => Promise<Source>;
  updateSource: (
    sourceId: string,
    data: Partial<SourceRequest>,
  ) => Promise<Source>;
  deleteSource: (sourceId: string) => Promise<void>;
}

export const createSourceAPISlice: StateCreator<
  SourceStore,
  [],
  [],
  SourceAPISlice
> = (_set, get) => ({
  fetchSources: async () => {
    const { setLoading, setError, setSources, sourceIds } = get();

    const hasExisting = sourceIds.length > 0;
    if (!hasExisting) setLoading(true);

    try {
      const data = await requestJSON<SourceList>(
        'GET',
        SOURCES_PATH,
        undefined,
        'Failed to fetch sources',
      );
      setSources(data.sources ?? null);
      setError(null);
    } catch (error) {
      setError(error instanceof Error ? error.message : 'Failed to fetch sources');
    } finally {
      if (!hasExisting) setLoading(false);
    }
  },

  listSources: async () => {
    const data = await requestJSON<SourceList>(
      'GET',
      SOURCES_PATH,
      undefined,
      'Failed to list sources',
    );
    return data.sources ?? [];
  },

  getSource: async (sourceId) => {
    return requestJSON<Source>(
      'GET',
      `${SOURCES_PATH}/${encodeURIComponent(sourceId)}`,
      undefined,
      'Failed to get source',
    );
  },

  createSource: async (request) => {
    const { setError, addSource } = get();
    try {
      const data = await requestJSON<Source>(
        'POST',
        SOURCES_PATH,
        request,
        'Failed to create source',
      );
      addSource(data);
      setError(null);
      return data;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Failed to create source';
      setError(message);
      throw error;
    }
  },

  updateSource: async (sourceId, data) => {
    const { setError, addSource } = get();
    try {
      const source = await requestJSON<Source>(
        'PATCH',
        `${SOURCES_PATH}/${encodeURIComponent(sourceId)}`,
        data,
        'Failed to update source',
      );
      addSource(source);
      setError(null);
      return source;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Failed to update source';
      setError(message);
      throw error;
    }
  },

  deleteSource: async (sourceId) => {
    const { setError, removeSource } = get();
    try {
      await requestJSON<void>(
        'DELETE',
        `${SOURCES_PATH}/${encodeURIComponent(sourceId)}`,
        undefined,
        'Failed to delete source',
      );
      removeSource(sourceId);
      setError(null);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Failed to delete source';
      setError(message);
      throw error;
    }
  },
});
