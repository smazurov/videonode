import { StateCreator } from 'zustand';

import { apiComposers, unwrap } from '../../../lib/api';
import type {
  Composer,
  ComposerRequest,
  LayoutSlot,
  Effect,
} from '../types';

import { ComposerStore } from '../../useComposerStore';

const COMPOSERS = '/api/composers';
const COMPOSER = '/api/composers/{id}';

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
      const data = unwrap(
        await apiComposers.GET(COMPOSERS),
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
    const data = unwrap(
      await apiComposers.GET(COMPOSERS),
      'Failed to list composers',
    );
    return data.composers ?? [];
  },

  getComposer: async (composerId) => {
    return unwrap(
      await apiComposers.GET(COMPOSER, {
        params: { path: { id: composerId } },
      }),
      'Failed to get composer',
    );
  },

  createComposer: async (request) => {
    const { addComposer } = get();
    const data = unwrap(
      await apiComposers.POST(COMPOSERS, { body: request }),
      'Failed to create composer',
    );
    addComposer(data);
    return data;
  },

  updateComposer: async (composerId, data) => {
    const { addComposer } = get();
    const composer = unwrap(
      await apiComposers.PATCH(COMPOSER, {
        params: { path: { id: composerId } },
        body: data,
      }),
      'Failed to update composer',
    );
    addComposer(composer);
    return composer;
  },

  deleteComposer: async (composerId) => {
    const { removeComposer } = get();
    unwrap(
      await apiComposers.DELETE(COMPOSER, {
        params: { path: { id: composerId } },
      }),
      'Failed to delete composer',
    );
    removeComposer(composerId);
  },

  updateComposerLayout: async (composerId, layout) => {
    const { addComposer } = get();
    const composer = unwrap(
      await apiComposers.PATCH('/api/composers/{id}/layout', {
        params: { path: { id: composerId } },
        body: { layout },
      }),
      'Failed to update composer layout',
    );
    addComposer(composer);
    return composer;
  },

  updateComposerInputEffect: async (composerId, inputRef, effect) => {
    const { addComposer } = get();
    const composer = unwrap(
      await apiComposers.PATCH('/api/composers/{id}/inputs/{ref}/effect', {
        params: { path: { id: composerId, ref: inputRef } },
        body: { effect },
      }),
      'Failed to update input effect',
    );
    addComposer(composer);
    return composer;
  },

  exportComposerToml: async (composerId) => {
    return unwrap(
      await apiComposers.GET('/api/composers/{id}/export', {
        params: { path: { id: composerId } },
        parseAs: 'text',
      }),
      'Failed to export composer',
    );
  },

  importComposerToml: async (toml) => {
    const { addComposer } = get();
    const composer = unwrap(
      await apiComposers.POST('/api/composers/import', {
        body: toml,
        bodySerializer: (body: string) => body,
        headers: { 'Content-Type': 'application/toml' },
      }),
      'Failed to import composer',
    );
    addComposer(composer);
    return composer;
  },

  importComposerTomlInto: async (composerId, toml) => {
    const { addComposer } = get();
    const composer = unwrap(
      await apiComposers.POST('/api/composers/{id}/import', {
        params: { path: { id: composerId } },
        body: toml,
        bodySerializer: (body: string) => body,
        headers: { 'Content-Type': 'application/toml' },
      }),
      'Failed to import composer',
    );
    addComposer(composer);
    return composer;
  },
});
