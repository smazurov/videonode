import { StateCreator } from 'zustand';
import { StreamData, StreamListData, SSEStreamMetricsEvent } from '../../lib/api';
import { StreamStore } from '../useStreamStore';

export interface StreamMetrics {
  fps?: string | undefined;
  dropped_frames?: string | undefined;
  duplicate_frames?: string | undefined;
}

export interface StreamDataSlice {
  streamIds: string[];
  streamsById: Record<string, StreamData>;
  metricsById: Record<string, StreamMetrics>;

  setStreams: (streamData: StreamListData) => void;
  addStream: (stream: StreamData) => void;
  removeStream: (streamId: string) => void;
  updateStreamMetrics: (metrics: SSEStreamMetricsEvent) => void;
  getStreamById: (streamId: string) => StreamData | undefined;
}

export const createStreamDataSlice: StateCreator<
  StreamStore,
  [],
  [],
  StreamDataSlice
> = (set, get) => ({
  streamIds: [],
  streamsById: {},
  metricsById: {},

  setStreams: (streamData) => {
    const ids: string[] = [];
    const byId: Record<string, StreamData> = {};
    const metricsById: Record<string, StreamMetrics> = {};
    for (const stream of streamData.streams) {
      ids.push(stream.stream_id);
      byId[stream.stream_id] = stream;
      metricsById[stream.stream_id] = {
        fps: stream.fps,
        dropped_frames: stream.dropped_frames,
        duplicate_frames: stream.duplicate_frames,
      };
    }
    set({
      streamIds: ids,
      streamsById: byId,
      metricsById,
      lastUpdated: new Date(),
    });
  },

  addStream: (stream) => {
    set((state) => {
      const isNew = !state.streamsById[stream.stream_id];
      return {
        streamIds: isNew
          ? [...state.streamIds, stream.stream_id]
          : state.streamIds,
        streamsById: { ...state.streamsById, [stream.stream_id]: stream },
        metricsById: {
          ...state.metricsById,
          [stream.stream_id]: {
            fps: stream.fps,
            dropped_frames: stream.dropped_frames,
            duplicate_frames: stream.duplicate_frames,
          },
        },
        lastUpdated: new Date(),
      };
    });
  },

  removeStream: (streamId) => {
    set((state) => {
      // eslint-disable-next-line @typescript-eslint/no-unused-vars, sonarjs/no-unused-vars
      const { [streamId]: _, ...restStreams } = state.streamsById;
      // eslint-disable-next-line @typescript-eslint/no-unused-vars, sonarjs/no-unused-vars
      const { [streamId]: __, ...restMetrics } = state.metricsById;
      return {
        streamIds: state.streamIds.filter(id => id !== streamId),
        streamsById: restStreams,
        metricsById: restMetrics,
        lastUpdated: new Date(),
      };
    });
  },

  updateStreamMetrics: (metrics) => {
    set((state) => {
      const existing = state.metricsById[metrics.stream_id];
      return {
        metricsById: {
          ...state.metricsById,
          [metrics.stream_id]: {
            ...existing,
            ...(metrics.fps !== undefined && { fps: metrics.fps }),
            ...(metrics.dropped_frames !== undefined && { dropped_frames: metrics.dropped_frames }),
            ...(metrics.duplicate_frames !== undefined && { duplicate_frames: metrics.duplicate_frames }),
          },
        },
      };
    });
  },

  getStreamById: (streamId) => get().streamsById[streamId],
});
