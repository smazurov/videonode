import { useEffect, useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import { useShallow } from 'zustand/shallow';
import { useAuthStore } from '../hooks/useAuthStore';
import { useStreamStore } from '../hooks/useStreamStore';
import { useSSEManager } from '../hooks/useSSEManager';
import { DashboardLayout } from '../components/DashboardLayout';
import { InfoBar } from '../components/InfoBar';
import { StreamsGrid } from '../components/StreamsGrid';
import {
  SSEStreamLifecycleEvent,
  SSEStreamMetricsEvent
} from '../lib/api';

export default function Streams() {
  const navigate = useNavigate();
  const { logout } = useAuthStore();

  // Use shallow comparison to prevent re-renders when streams Map reference changes but content is same
  const { loading, error, streams } = useStreamStore(
    useShallow((state) => ({
      loading: state.loading,
      error: state.error,
      streams: state.streams,
    }))
  );
  const { fetchStreams, deleteStream, addStreamFromSSE, removeStreamFromSSE } = useStreamStore();

  // Setup SSE listener for stream lifecycle and metrics events
  useSSEManager({
    onStreamLifecycleEvent: (event: SSEStreamLifecycleEvent) => {
      console.log('Received SSE stream lifecycle event:', event);
      
      if (event.type === 'stream-created') {
        addStreamFromSSE(event.stream);
      } else if (event.type === 'stream-updated') {
        addStreamFromSSE(event.stream); // This will update existing stream due to deduplication
      } else if (event.type === 'stream-deleted') {
        removeStreamFromSSE(event.stream_id);
      }
    },
    onStreamMetricsEvent: (_event: SSEStreamMetricsEvent) => {
      // Disabled: frequent updates cause re-renders that break button clicks
      // updateStreamMetrics(event);
    }
  });

  // Load streams on mount
  useEffect(() => {
    fetchStreams();
  }, [fetchStreams]);

  const handleDeleteStream = async (streamId: string) => {
    try {
      await deleteStream(streamId);
      // SSE event will update the UI
    } catch (error) {
      console.error('Failed to delete stream:', error);
      throw error;
    }
  };

  const handleCreateStream = () => {
    navigate('/streams/new');
  };

  const handleLogout = () => {
    logout();
  };



  // Memoize streams array to prevent re-renders on every SSE update
  const streamsArray = useMemo(() => Array.from(streams.values()), [streams]);

  // Bottom bar content - using InfoBar component
  const bottomBar = <InfoBar />;

  return (
    <DashboardLayout
      onLogout={handleLogout}
      bottomBar={bottomBar}
    >
      <DashboardLayout.MainContent>
        <StreamsGrid
          streams={streamsArray}
          loading={loading}
          error={error}
          onRefresh={fetchStreams}
          onDeleteStream={handleDeleteStream}
          onCreateStream={handleCreateStream}
        />
      </DashboardLayout.MainContent>
    </DashboardLayout>
  );
}