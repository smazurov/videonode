import { StateCreator } from 'zustand';
import { 
  StreamRequestData, 
  StreamData, 
  getStreams, 
  createStream, 
  deleteStream 
} from '../../lib/api';
import { StreamStore } from '../useStreamStore';

export interface APISlice {
  fetchStreams: () => Promise<void>;
  createStream: (request: StreamRequestData) => Promise<StreamData>;
  deleteStream: (streamId: string) => Promise<void>;
}

export const createAPISlice: StateCreator<
  StreamStore,
  [],
  [],
  APISlice
> = (_set, get) => ({
  fetchStreams: async () => {
    const { setLoading, setError, setStreams } = get();
    setLoading(true);
    
    try {
      const data = await getStreams();
      setStreams(data);
      setError(null);
    } catch (error) {
      setError(error instanceof Error ? error.message : 'Failed to fetch streams');
    } finally {
      setLoading(false);
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