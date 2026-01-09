import { StateCreator } from 'zustand';
import { StreamData, StreamListData, SSEStreamMetricsEvent } from '../../lib/api';
import { StreamStore } from '../useStreamStore';

export interface StreamDataSlice {
  streamIds: string[];
  streamsById: Record<string, StreamData>;

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

  setStreams: (streamData) => {
    const ids: string[] = [];
    const byId: Record<string, StreamData> = {};
    for (const stream of streamData.streams) {
      ids.push(stream.stream_id);
      byId[stream.stream_id] = stream;
    }
    set({
      streamIds: ids,
      streamsById: byId,
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
        lastUpdated: new Date(),
      };
    });
  },

  removeStream: (streamId) => {
    set((state) => {
      // eslint-disable-next-line @typescript-eslint/no-unused-vars, sonarjs/no-unused-vars
      const { [streamId]: _, ...rest } = state.streamsById;
      return {
        streamIds: state.streamIds.filter(id => id !== streamId),
        streamsById: rest,
        lastUpdated: new Date(),
      };
    });
  },

  updateStreamMetrics: (metrics) => {
    set((state) => {
      const stream = state.streamsById[metrics.stream_id];
      if (!stream) return state;
      return {
        streamsById: {
          ...state.streamsById,
          [metrics.stream_id]: {
            ...stream,
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
