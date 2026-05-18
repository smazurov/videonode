import { StateCreator } from 'zustand';
import type { components } from '../../lib/api.generated';
import { api, unwrap } from '../../lib/api';
import { StreamStore } from '../useStreamStore';

type StreamRequestData = components["schemas"]["StreamRequestData"];
type StreamData = components["schemas"]["StreamData"];

export interface APISlice {
  fetchStreams: () => Promise<void>;
  createStream: (request: StreamRequestData) => Promise<StreamData>;
  updateStream: (streamId: string, data: Partial<StreamRequestData>) => Promise<StreamData>;
  deleteStream: (streamId: string) => Promise<void>;
  releaseCanvas: (streamId: string) => Promise<void>;
  engageCanvas: (streamId: string) => Promise<void>;
}

export const createAPISlice: StateCreator<
  StreamStore,
  [],
  [],
  APISlice
> = (_set, get) => ({
  fetchStreams: async () => {
    const { setLoading, setError, setStreams, streamIds } = get();

    const hasExistingStreams = streamIds.length > 0;
    if (!hasExistingStreams) {
      setLoading(true);
    }

    try {
      const data = unwrap(await api.GET("/api/streams"), 'Failed to fetch streams');
      setStreams(data);
      setError(null);
    } catch (error) {
      setError(error instanceof Error ? error.message : 'Failed to fetch streams');
    } finally {
      if (!hasExistingStreams) {
        setLoading(false);
      }
    }
  },

  createStream: async (request) => {
    const { setError } = get();

    try {
      const data = unwrap(
        await api.POST("/api/streams", { body: request }),
        'Failed to create stream',
      );
      setError(null);
      return data;
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
        await api.PATCH("/api/streams/{stream_id}", {
          params: { path: { stream_id: streamId } },
          body: data,
        }),
        'Failed to update stream',
      );
      setError(null);
      return stream;
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
        await api.DELETE("/api/streams/{stream_id}", {
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

  releaseCanvas: async (streamId) => {
    const { setError } = get();

    try {
      unwrap(
        await api.POST("/api/streams/{stream_id}/canvas/release", {
          params: { path: { stream_id: streamId } },
        }),
        'Failed to release canvas',
      );
      setError(null);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Failed to release canvas';
      setError(message);
      throw error;
    }
  },

  engageCanvas: async (streamId) => {
    const { setError } = get();

    try {
      unwrap(
        await api.POST("/api/streams/{stream_id}/canvas/engage", {
          params: { path: { stream_id: streamId } },
        }),
        'Failed to engage canvas',
      );
      setError(null);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Failed to engage canvas';
      setError(message);
      throw error;
    }
  },
});
