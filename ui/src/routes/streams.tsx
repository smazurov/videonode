import { useEffect, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import { useShallow } from 'zustand/shallow';
import toast from 'react-hot-toast';
import { useAuthStore } from '../hooks/useAuthStore';
import { useStreamStore } from '../hooks/useStreamStore';
import { useSSEManager } from '../hooks/useSSEManager';
import { DashboardLayout } from '../components/DashboardLayout';
import { InfoBar } from '../components/InfoBar';
import { StreamsGrid } from '../components/StreamsGrid';
import type { components } from '../lib/api.generated';
import type { StreamLifecycleEvent } from '../hooks/useSSEManager';

type StreamMetricsEvent = components["schemas"]["StreamMetricsEvent"];

export default function Streams() {
  const navigate = useNavigate();
  const { logout } = useAuthStore();

  // Subscribe to stream IDs only - won't re-render on data updates
  const streamIds = useStreamStore((state) => state.streamIds);

  // Separate selector for loading/error state
  const { loading, error } = useStreamStore(
    useShallow((state) => ({
      loading: state.loading,
      error: state.error,
    }))
  );

  // Get actions without subscribing to state changes
  const fetchStreams = useStreamStore((state) => state.fetchStreams);
  const deleteStream = useStreamStore((state) => state.deleteStream);
  const addStream = useStreamStore((state) => state.addStream);
  const removeStream = useStreamStore((state) => state.removeStream);
  const updateStreamMetrics = useStreamStore((state) => state.updateStreamMetrics);
  const bumpStreamRefreshKey = useStreamStore((state) => state.bumpStreamRefreshKey);

  const handleStreamLifecycle = useCallback((event: StreamLifecycleEvent) => {
    if (event.type === 'stream-created') {
      addStream(event.stream);
    } else if (event.type === 'stream-updated') {
      const currentStream = useStreamStore.getState().streamsById[event.stream.stream_id];
      addStream(event.stream);

      if (event.action === 'restarted') {
        toast.success(`Stream '${event.stream.stream_id}' has restarted`);
      } else if (currentStream && currentStream.test_mode !== event.stream.test_mode) {
        toast.success(`Test mode ${event.stream.test_mode ? 'enabled' : 'disabled'}`);
      }
    } else if (event.type === 'stream-deleted') {
      removeStream(event.stream_id);
    } else if (event.type === 'canvas-restarted') {
      if (event.canvas) {
        addStream(event.canvas);
      }
      bumpStreamRefreshKey(event.canvas_id);
    }
  }, [addStream, removeStream, bumpStreamRefreshKey]);

  const handleStreamMetrics = useCallback((event: StreamMetricsEvent) => {
    updateStreamMetrics(event);
  }, [updateStreamMetrics]);

  // Setup SSE listener for stream lifecycle and metrics events
  useSSEManager({
    onStreamLifecycleEvent: handleStreamLifecycle,
    onStreamMetricsEvent: handleStreamMetrics,
  });

  // Load streams on mount
  useEffect(() => {
    fetchStreams();
  }, [fetchStreams]);

  const handleDeleteStream = useCallback(async (streamId: string) => {
    try {
      await deleteStream(streamId);
    } catch (error) {
      console.error('Failed to delete stream:', error);
      throw error;
    }
  }, [deleteStream]);

  const handleCreateStream = useCallback(() => {
    navigate('/streams/new');
  }, [navigate]);

  const handleCreateCanvas = useCallback(() => {
    navigate('/streams/canvas/new');
  }, [navigate]);

  const handleLogout = useCallback(() => {
    logout();
  }, [logout]);

  // Bottom bar content - using InfoBar component
  const bottomBar = <InfoBar />;

  return (
    <DashboardLayout
      onLogout={handleLogout}
      bottomBar={bottomBar}
    >
      <DashboardLayout.MainContent>
        <StreamsGrid
          streamIds={streamIds}
          loading={loading}
          error={error}
          onRefresh={fetchStreams}
          onDeleteStream={handleDeleteStream}
          onCreateStream={handleCreateStream}
          onCreateCanvas={handleCreateCanvas}
        />
      </DashboardLayout.MainContent>
    </DashboardLayout>
  );
}
