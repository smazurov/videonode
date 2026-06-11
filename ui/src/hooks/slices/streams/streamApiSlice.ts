import { StateCreator } from 'zustand';

import { api, unwrap } from '../../../lib/api';
import type { Stream } from '../types';
import { StreamStore } from '../../useStreamStore';

// Use existing generated request/response shapes until U1's regen ships
// the new slim shape. The slice transparently re-types the response as
// Stream so consumers see the merged canonical+legacy bag.
import type { components } from '../../../lib/api.generated';

type StreamRequestData = components['schemas']['StreamRequestData'];
type RecordingStatusData = components['schemas']['RecordingStatusData'];

const STREAMS_PATH = '/api/streams' as const;
const STREAM_ID_PATH = '/api/streams/{stream_id}' as const;
const RECORDING_PATH = '/api/streams/{stream_id}/recording' as const;

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
  startRecording: (streamId: string) => Promise<RecordingStatusData>;
  stopRecording: (streamId: string) => Promise<RecordingStatusData>;
  getRecording: (streamId: string) => Promise<RecordingStatusData | null>;
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
    const data = unwrap(
      await api.POST(STREAMS_PATH, { body: request }),
      'Failed to create stream',
    );
    return data as Stream;
  },

  updateStream: async (streamId, data) => {
    const stream = unwrap(
      await api.PATCH(STREAM_ID_PATH, {
        params: { path: { stream_id: streamId } },
        body: data,
      }),
      'Failed to update stream',
    );
    return stream as Stream;
  },

  deleteStream: async (streamId) => {
    unwrap(
      await api.DELETE(STREAM_ID_PATH, {
        params: { path: { stream_id: streamId } },
      }),
      'Failed to delete stream',
    );
  },

  startRecording: async (streamId) => {
    const data = unwrap(
      await api.POST(RECORDING_PATH, {
        params: { path: { stream_id: streamId } },
      }),
      'Failed to start recording',
    );
    return data as RecordingStatusData;
  },

  stopRecording: async (streamId) => {
    const data = unwrap(
      await api.DELETE(RECORDING_PATH, {
        params: { path: { stream_id: streamId } },
      }),
      'Failed to stop recording',
    );
    return data as RecordingStatusData;
  },

  // getRecording returns null when the stream isn't recording (404), so a
  // missing recording is a normal state rather than a thrown error.
  getRecording: async (streamId) => {
    const { data, error } = await api.GET(RECORDING_PATH, {
      params: { path: { stream_id: streamId } },
    });
    if (error) return null;
    return data as RecordingStatusData;
  },
});
