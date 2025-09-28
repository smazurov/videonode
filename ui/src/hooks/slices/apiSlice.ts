import { StateCreator } from 'zustand';
import { 
  StreamRequestData, 
  StreamData, 
  getStreams, 
  createStream, 
  updateStream,
  deleteStream 
} from '../../lib/api';
import { StreamStore } from '../useStreamStore';

export interface APISlice {
  fetchStreams: () => Promise<void>;
  createStream: (request: StreamRequestData) => Promise<StreamData>;
  updateStream: (streamId: string, data: Partial<StreamRequestData>) => Promise<StreamData>;
  deleteStream: (streamId: string) => Promise<void>;
}

export const createAPISlice: StateCreator<
  StreamStore,
  [],
  [],
  APISlice
> = (_set, get) => ({
  fetchStreams: async () => {
    const { setLoading, setError, setStreams, streams } = get();
    
    // Only show loading if we don't have any streams yet (initial load)
    const hasExistingStreams = streams.size > 0;
    if (!hasExistingStreams) {
      setLoading(true);
    }
    
    try {
      const data = await getStreams();
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
    const { setLoading, setError, addStreamFromPost } = get();
    setLoading(true);
    
    try {
      const stream = await createStream(request);
      addStreamFromPost(stream);
      setError(null);
      return stream;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Failed to create stream';
      setError(message);
      throw error;
    } finally {
      setLoading(false);
    }
  },
  
  updateStream: async (streamId, data) => {
    const { setLoading, setError, addStream } = get();
    setLoading(true);
    
    try {
      const stream = await updateStream(streamId, data);
      addStream(stream); // Updates existing stream in memory
      setError(null);
      return stream;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Failed to update stream';
      setError(message);
      throw error;
    } finally {
      setLoading(false);
    }
  },
  
  deleteStream: async (streamId) => {
    const { setLoading, setError, removeStreamFromPost } = get();
    setLoading(true);
    
    try {
      await deleteStream(streamId);
      removeStreamFromPost(streamId);
      setError(null);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Failed to delete stream';
      setError(message);
      throw error;
    } finally {
      setLoading(false);
    }
  },
});