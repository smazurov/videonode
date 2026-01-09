import { StateCreator } from 'zustand';
import { StreamData } from '../../lib/api';
import { StreamStore } from '../useStreamStore';

interface StreamOperation {
  streamId: string;
  type: 'create' | 'delete';
  timestamp: number;
}

export interface DeduplicationSlice {
  recentOperations: Map<string, StreamOperation>;
  
  addStreamFromPost: (stream: StreamData) => void;
  addStreamFromSSE: (stream: StreamData) => void;
  removeStreamFromPost: (streamId: string) => void;
  removeStreamFromSSE: (streamId: string) => void;
}

const DEDUP_WINDOW_MS = 2000; // 2 second window for deduplication

export const createDeduplicationSlice: StateCreator<
  StreamStore,
  [],
  [],
  DeduplicationSlice
> = (set, get) => ({
  recentOperations: new Map<string, StreamOperation>(),
  
  addStreamFromPost: (stream) => {
    const { recentOperations, addStream } = get();
    
    // Record this operation for deduplication
    const operation: StreamOperation = {
      streamId: stream.stream_id,
      type: 'create',
      timestamp: Date.now(),
    };
    
    const newOperations = new Map(recentOperations);
    newOperations.set(stream.stream_id, operation);
    
    // Add to streams
    addStream(stream);
    
    // Update recent operations
    set({ recentOperations: newOperations });
    
    // Schedule cleanup
    setTimeout(() => {
      const currentOps = get().recentOperations;
      const updatedOps = new Map(currentOps);
      const op = updatedOps.get(stream.stream_id);
      
      // Only delete if it's the same operation (not replaced by a newer one)
      if (op && op.timestamp === operation.timestamp) {
        updatedOps.delete(stream.stream_id);
        set({ recentOperations: updatedOps });
      }
    }, DEDUP_WINDOW_MS);
  },
  
  addStreamFromSSE: (stream) => {
    const { recentOperations, addStream, streamsById } = get();

    // Check for recent POST operation
    const recentOp = recentOperations.get(stream.stream_id);
    if (recentOp &&
        recentOp.type === 'create' &&
        (Date.now() - recentOp.timestamp) < DEDUP_WINDOW_MS) {
      // Skip duplicate - this is an SSE echo of our POST
      return;
    }

    // Check if stream data actually changed (avoid unnecessary re-renders)
    const existingStream = streamsById[stream.stream_id];
    if (existingStream && JSON.stringify(existingStream) === JSON.stringify(stream)) {
      return; // No change, skip update
    }

    // Not a duplicate and data changed, add the stream
    addStream(stream);
  },
  
  removeStreamFromPost: (streamId) => {
    const { recentOperations, removeStream } = get();
    
    // Record this operation for deduplication
    const operation: StreamOperation = {
      streamId,
      type: 'delete',
      timestamp: Date.now(),
    };
    
    const newOperations = new Map(recentOperations);
    newOperations.set(streamId, operation);
    
    // Remove from streams
    removeStream(streamId);
    
    // Update recent operations
    set({ recentOperations: newOperations });
    
    // Schedule cleanup
    setTimeout(() => {
      const currentOps = get().recentOperations;
      const updatedOps = new Map(currentOps);
      const op = updatedOps.get(streamId);
      
      // Only delete if it's the same operation
      if (op && op.timestamp === operation.timestamp) {
        updatedOps.delete(streamId);
        set({ recentOperations: updatedOps });
      }
    }, DEDUP_WINDOW_MS);
  },
  
  removeStreamFromSSE: (streamId) => {
    const { recentOperations, removeStream } = get();
    
    // Check for recent DELETE operation
    const recentOp = recentOperations.get(streamId);
    if (recentOp && 
        recentOp.type === 'delete' && 
        (Date.now() - recentOp.timestamp) < DEDUP_WINDOW_MS) {
      // Skip duplicate - this is an SSE echo of our DELETE
      return;
    }
    
    // Not a duplicate, remove the stream
    removeStream(streamId);
  },
});