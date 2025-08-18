import { StateCreator } from 'zustand';
import { StreamData, StreamListData, SSEStreamMetricsEvent } from '../../lib/api';
import { StreamStore } from '../useStreamStore';

export interface StreamDataSlice {
  streams: Map<string, StreamData>;
  
  // Core data operations
  setStreams: (streamData: StreamListData) => void;
  addStream: (stream: StreamData) => void;
  removeStream: (streamId: string) => void;
  updateStreamMetrics: (metrics: SSEStreamMetricsEvent) => void;
  getStreamsArray: () => StreamData[];
}

export const createStreamDataSlice: StateCreator<
  StreamStore,
  [],
  [],
  StreamDataSlice
> = (set, get) => ({
  streams: new Map<string, StreamData>(),
  
  setStreams: (streamData) => {
    const streamMap = new Map<string, StreamData>();
    for (const stream of streamData.streams) {
      streamMap.set(stream.stream_id, stream);
    }
    set({ 
      streams: streamMap,
      lastUpdated: new Date(),
    });
  },
  
  addStream: (stream) => {
    set((state) => {
      const newStreams = new Map(state.streams);
      newStreams.set(stream.stream_id, stream);
      return { 
        streams: newStreams,
        lastUpdated: new Date(),
      };
    });
  },
  
  removeStream: (streamId) => {
    set((state) => {
      const newStreams = new Map(state.streams);
      newStreams.delete(streamId);
      return { 
        streams: newStreams,
        lastUpdated: new Date(),
      };
    });
  },
  
  updateStreamMetrics: (metrics) => {
    set((state) => {
      const stream = state.streams.get(metrics.stream_id);
      if (stream) {
        const newStreams = new Map(state.streams);
        const updatedStream: StreamData = {
          ...stream,
          metrics: {
            fps: metrics.fps,
            dropped_frames: metrics.dropped_frames,
            duplicate_frames: metrics.duplicate_frames,
            processing_speed: metrics.processing_speed,
            timestamp: metrics.timestamp,
          }
        };
        newStreams.set(metrics.stream_id, updatedStream);
        return { streams: newStreams };
      }
      return state;
    });
  },
  
  getStreamsArray: () => {
    return Array.from(get().streams.values());
  },
});