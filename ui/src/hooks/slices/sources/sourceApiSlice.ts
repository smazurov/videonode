import { StateCreator } from 'zustand';

import { apiSources, unwrap } from '../../../lib/api';
import type { Source, SourceRequestData as SourceRequest } from '../types';

import { SourceStore } from '../../useSourceStore';

const SOURCES = '/api/sources';
const SOURCE = '/api/sources/{source_id}';

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
      const data = unwrap(
        await apiSources.GET(SOURCES),
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
    const data = unwrap(
      await apiSources.GET(SOURCES),
      'Failed to list sources',
    );
    return data.sources ?? [];
  },

  getSource: async (sourceId) => {
    return unwrap(
      await apiSources.GET(SOURCE, {
        params: { path: { source_id: sourceId } },
      }),
      'Failed to get source',
    );
  },

  createSource: async (request) => {
    const { addSource } = get();
    const data = unwrap(
      await apiSources.POST(SOURCES, { body: request }),
      'Failed to create source',
    );
    addSource(data);
    return data;
  },

  updateSource: async (sourceId, data) => {
    const { addSource } = get();
    const source = unwrap(
      await apiSources.PATCH(SOURCE, {
        params: { path: { source_id: sourceId } },
        body: data,
      }),
      'Failed to update source',
    );
    addSource(source);
    return source;
  },

  deleteSource: async (sourceId) => {
    const { removeSource } = get();
    unwrap(
      await apiSources.DELETE(SOURCE, {
        params: { path: { source_id: sourceId } },
      }),
      'Failed to delete source',
    );
    removeSource(sourceId);
  },
});
