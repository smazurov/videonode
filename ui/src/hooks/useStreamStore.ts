import { create } from 'zustand';
import { StreamData, StreamListData, getStreams, createStream, deleteStream, StreamRequestData, SSEStreamMetricsEvent } from '../lib/api';

interface StreamOperation {
  streamId: string;
  type: 'create' | 'delete';
  timestamp: number;
}

interface StreamStore {
  streams: Map<string, StreamData>;
  recentOperations: Map<string, StreamOperation>;
  loading: boolean;
  error: string | null;
  lastUpdated: Date | null;
  
  // Actions
  setStreams: (streamData: StreamListData) => void;
  setLoading: (loading: boolean) => void;
  setError: (error: string | null) => void;
  
  // Stream operations with deduplication
  addStreamFromPost: (stream: StreamData) => void;
  addStreamFromSSE: (stream: StreamData) => void;
  removeStreamFromPost: (streamId: string) => void;
  removeStreamFromSSE: (streamId: string) => void;
  updateStreamMetrics: (metrics: SSEStreamMetricsEvent) => void;
  
  // API operations
  fetchStreams: () => Promise<void>;
  createStream: (request: StreamRequestData) => Promise<StreamData>;
  deleteStream: (streamId: string) => Promise<void>;
  
  // Utility
  getStreamsArray: () => StreamData[];
  reset: () => void;
}

const DEDUP_WINDOW_MS = 2000; // 2 second window for deduplication

const initialState = {
  streams: new Map<string, StreamData>(),
  recentOperations: new Map<string, StreamOperation>(),
  loading: false,
  error: null,
  lastUpdated: null,
};

export const useStreamStore = create<StreamStore>((set, get) => ({
  ...initialState,
  
  setStreams: (streamData: StreamListData) => {
    const streamMap = new Map<string, StreamData>();
    streamData.streams.forEach(stream => {
      streamMap.set(stream.stream_id, stream);
    });
    
    set({
      streams: streamMap,
      loading: false,
      error: null,
      lastUpdated: new Date(),
    });
  },
  
  setLoading: (loading: boolean) => {
    set({ loading });
  },
  
  setError: (error: string | null) => {
    set({ error, loading: false });
  },
  
  addStreamFromPost: (stream: StreamData) => {
    const { streams, recentOperations } = get();
    
    // Record this operation for deduplication
    const operation: StreamOperation = {
      streamId: stream.stream_id,
      type: 'create',
      timestamp: Date.now(),
    };
    
    const newOperations = new Map(recentOperations);
    newOperations.set(stream.stream_id, operation);
    
    // Clean up old operations
    Array.from(newOperations.entries()).forEach(([id, op]) => {
      if (Date.now() - op.timestamp > DEDUP_WINDOW_MS) {
        newOperations.delete(id);
      }
    });
    
    const newStreams = new Map(streams);
    newStreams.set(stream.stream_id, stream);
    
    set({
      streams: newStreams,
      recentOperations: newOperations,
      lastUpdated: new Date(),
    });
  },
  
  addStreamFromSSE: (stream: StreamData) => {
    const { streams, recentOperations } = get();
    
    // Check if this is a recent operation we already handled
    const recentOp = recentOperations.get(stream.stream_id);
    if (recentOp && recentOp.type === 'create' && 
        Date.now() - recentOp.timestamp < DEDUP_WINDOW_MS) {
      console.log(`Skipping duplicate SSE create for stream ${stream.stream_id}`);
      return;
    }
    
    const newStreams = new Map(streams);
    newStreams.set(stream.stream_id, stream);
    
    set({
      streams: newStreams,
      lastUpdated: new Date(),
    });
  },
  
  removeStreamFromPost: (streamId: string) => {
    const { streams, recentOperations } = get();
    
    // Record this operation for deduplication
    const operation: StreamOperation = {
      streamId,
      type: 'delete',
      timestamp: Date.now(),
    };
    
    const newOperations = new Map(recentOperations);
    newOperations.set(streamId, operation);
    
    // Clean up old operations
    Array.from(newOperations.entries()).forEach(([id, op]) => {
      if (Date.now() - op.timestamp > DEDUP_WINDOW_MS) {
        newOperations.delete(id);
      }
    });
    
    const newStreams = new Map(streams);
    newStreams.delete(streamId);
    
    set({
      streams: newStreams,
      recentOperations: newOperations,
      lastUpdated: new Date(),
    });
  },
  
  removeStreamFromSSE: (streamId: string) => {
    const { streams, recentOperations } = get();
    
    // Check if this is a recent operation we already handled
    const recentOp = recentOperations.get(streamId);
    if (recentOp && recentOp.type === 'delete' && 
        Date.now() - recentOp.timestamp < DEDUP_WINDOW_MS) {
      console.log(`Skipping duplicate SSE delete for stream ${streamId}`);
      return;
    }
    
    const newStreams = new Map(streams);
    newStreams.delete(streamId);
    
    set({
      streams: newStreams,
      lastUpdated: new Date(),
    });
  },
  
  updateStreamMetrics: (metrics: SSEStreamMetricsEvent) => {
    const { streams } = get();
    const existingStream = streams.get(metrics.stream_id);
    
    if (!existingStream) {
      console.log(`Stream ${metrics.stream_id} not found for metrics update`);
      return;
    }
    
    const updatedStream: StreamData = {
      ...existingStream,
      fps: metrics.fps,
      dropped_frames: metrics.dropped_frames,
      duplicate_frames: metrics.duplicate_frames,
      processing_speed: metrics.processing_speed,
    };
    
    const newStreams = new Map(streams);
    newStreams.set(metrics.stream_id, updatedStream);
    
    set({
      streams: newStreams,
      lastUpdated: new Date(),
    });
  },
  
  fetchStreams: async () => {
    const { setLoading, setStreams, setError } = get();
    
    try {
      setLoading(true);
      setError(null);
      const streamData = await getStreams();
      setStreams(streamData);
    } catch (error) {
      console.error('Failed to fetch streams:', error);
      setError(error instanceof Error ? error.message : 'Failed to fetch streams');
    }
  },
  
  createStream: async (request: StreamRequestData) => {
    const { addStreamFromPost, setError } = get();
    
    try {
      setError(null);
      const newStream = await createStream(request);
      addStreamFromPost(newStream);
      return newStream;
    } catch (error) {
      console.error('Failed to create stream:', error);
      setError(error instanceof Error ? error.message : 'Failed to create stream');
      throw error;
    }
  },
  
  deleteStream: async (streamId: string) => {
    const { removeStreamFromPost, setError } = get();
    
    try {
      setError(null);
      await deleteStream(streamId);
      removeStreamFromPost(streamId);
    } catch (error) {
      console.error('Failed to delete stream:', error);
      setError(error instanceof Error ? error.message : 'Failed to delete stream');
      throw error;
    }
  },
  
  getStreamsArray: () => {
    const { streams } = get();
    return Array.from(streams.values());
  },
  
  reset: () => {
    set(initialState);
  },
}));