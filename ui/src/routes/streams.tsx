import { useCallback, useEffect, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import { useShallow } from 'zustand/shallow';
import toast from 'react-hot-toast';
import { useAuthStore } from '../hooks/useAuthStore';
import { useStreamStore } from '../hooks/useStreamStore';
import { useSSEManager } from '../hooks/useSSEManager';
import type { StreamLifecycleEvent } from '../hooks/useSSEManager';
import { DashboardLayout } from '../components/DashboardLayout';
import { InfoBar } from '../components/InfoBar';
import { StreamList } from '../components/streams/StreamList';
import type { components } from '../lib/api.generated';

type StreamMetricsEvent = components['schemas']['StreamMetricsEvent'];

export default function Streams() {
  const navigate = useNavigate();
  const { logout } = useAuthStore();

  const streamIds = useStreamStore((state) => state.streamIds);
  const { loading, error } = useStreamStore(
    useShallow((state) => ({
      loading: state.loading,
      error: state.error,
    })),
  );

  const fetchStreams = useStreamStore((state) => state.fetchStreams);
  const addStream = useStreamStore((state) => state.addStream);
  const removeStream = useStreamStore((state) => state.removeStream);
  const updateStreamMetrics = useStreamStore((state) => state.updateStreamMetrics);

  const handleStreamLifecycle = useCallback(
    (event: StreamLifecycleEvent) => {
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
      }
    },
    [addStream, removeStream],
  );

  const handleStreamMetrics = useCallback(
    (event: StreamMetricsEvent) => {
      updateStreamMetrics(event);
    },
    [updateStreamMetrics],
  );

  // SSE drops can hide release/engage and create/delete events that happen
  // mid-disconnect; refetch on every transition into 'online' so the list
  // always reflects server truth.
  const prevConnectionStatusRef = useRef<'online' | 'offline' | 'reconnecting'>('online');
  const handleConnectionStatus = useCallback(
    (status: 'online' | 'offline' | 'reconnecting') => {
      if (status === 'online' && prevConnectionStatusRef.current !== 'online') {
        fetchStreams();
      }
      prevConnectionStatusRef.current = status;
    },
    [fetchStreams],
  );

  useSSEManager({
    onStreamLifecycleEvent: handleStreamLifecycle,
    onStreamMetricsEvent: handleStreamMetrics,
    onConnectionStatusChange: handleConnectionStatus,
  });

  useEffect(() => {
    fetchStreams();
  }, [fetchStreams]);

  const handleCreateStream = useCallback(() => {
    navigate('/streams/new');
  }, [navigate]);

  const handleLogout = useCallback(() => {
    logout();
  }, [logout]);

  return (
    <DashboardLayout onLogout={handleLogout} bottomBar={<InfoBar />}>
      <DashboardLayout.MainContent>
        <StreamList
          streamIds={streamIds}
          loading={loading}
          error={error}
          onRefresh={fetchStreams}
          onCreateStream={handleCreateStream}
        />
      </DashboardLayout.MainContent>
    </DashboardLayout>
  );
}
