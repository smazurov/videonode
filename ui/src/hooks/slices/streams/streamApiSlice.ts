import { StateCreator } from 'zustand';

import { api, unwrap } from '../../../lib/api';
import type { Stream } from '../types';
import { StreamStore } from '../../useStreamStore';

// Use existing generated request/response shapes until U1's regen ships
// the new slim shape. The slice transparently re-types the response as
// Stream so consumers see the merged canonical+legacy bag.
import type { components } from '../../../lib/api.generated';

type StreamRequestData = components['schemas']['StreamRequestData'];

const STREAMS_PATH = '/api/streams' as const;
const STREAM_ID_PATH = '/api/streams/{stream_id}' as const;

export interface StreamAPISlice {
  fetchStreams: () => Promise<void>;
  listStreams: () => Promise<Stream[]>;
  getStream: (streamId: string) => Promise<Stream>;
  createStream: (request: StreamRequestData) => Promise<Stream>;
  updateStream: (
    streamId: string,
    data: Partial<StreamRequestData>,
  ) => Promise<Stream>;
  deleteStream: (streamId: string) => Promise<void>;
}

export const createStreamAPISlice: StateCreator<
  StreamStore,
  [],
  [],
  StreamAPISlice
> = (_set, get) => ({
  fetchStreams: async () => {
    const { setLoading, setError, setStreams, streamIds } = get();

    const hasExistingStreams = streamIds.length > 0;
    if (!hasExistingStreams) {
      setLoading(true);
    }

    try {
      const data = unwrap(
        await api.GET(STREAMS_PATH),
        'Failed to fetch streams',
      );
      setStreams((data.streams as Stream[] | null | undefined) ?? null);
      setError(null);
    } catch (error) {
      setError(error instanceof Error ? error.message : 'Failed to fetch streams');
    } finally {
      if (!hasExistingStreams) {
        setLoading(false);
      }
    }
  },

  listStreams: async () => {
    const data = unwrap(
      await api.GET(STREAMS_PATH),
      'Failed to list streams',
    );
    return (data.streams as Stream[] | null | undefined) ?? [];
  },

  getStream: async (streamId) => {
    const data = unwrap(
      await api.GET(STREAM_ID_PATH, {
        params: { path: { stream_id: streamId } },
      }),
      'Failed to get stream',
    );
    return data as Stream;
  },

  createStream: async (request) => {
    const { setError } = get();

    try {
      const data = unwrap(
        await api.POST(STREAMS_PATH, { body: request }),
        'Failed to create stream',
      );
      setError(null);
      return data as Stream;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Failed to create stream';
      setError(message);
      throw error;
    }
  },

  updateStream: async (streamId, data) => {
    const { setError } = get();

    try {
      const stream = unwrap(
        await api.PATCH(STREAM_ID_PATH, {
          params: { path: { stream_id: streamId } },
          body: data,
        }),
        'Failed to update stream',
      );
      setError(null);
      return stream as Stream;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Failed to update stream';
      setError(message);
      throw error;
    }
  },

  deleteStream: async (streamId) => {
    const { setError } = get();

    try {
      unwrap(
        await api.DELETE(STREAM_ID_PATH, {
          params: { path: { stream_id: streamId } },
        }),
        'Failed to delete stream',
      );
      setError(null);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Failed to delete stream';
      setError(message);
      throw error;
    }
  },
});
