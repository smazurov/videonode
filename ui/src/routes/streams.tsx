import { useCallback, useEffect, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import { useShallow } from 'zustand/shallow';
import { useAuthStore } from '../hooks/useAuthStore';
import { useStreamStore } from '../hooks/useStreamStore';
import { useSSEManager } from '../hooks/useSSEManager';
import { DashboardLayout } from '../components/DashboardLayout';
import { InfoBar } from '../components/InfoBar';
import { StreamList } from '../components/streams/StreamList';

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
