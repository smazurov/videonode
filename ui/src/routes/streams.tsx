import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
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
  const { loading, error, fetchStreams, deleteStream, getStreamsArray, addStreamFromSSE, removeStreamFromSSE, updateStreamMetrics } = useStreamStore();
  const [viewMode, setViewMode] = useState<'grid' | 'tabs'>('grid');

  // Setup SSE listener for stream lifecycle and metrics events
  useSSEManager({
    onStreamLifecycleEvent: (event: SSEStreamLifecycleEvent) => {
      console.log('Received SSE stream lifecycle event:', event);
      
      if (event.type === 'stream-created') {
        addStreamFromSSE(event.stream);
      } else if (event.type === 'stream-deleted') {
        removeStreamFromSSE(event.stream_id);
      }
    },
    onStreamMetricsEvent: (event: SSEStreamMetricsEvent) => {
      updateStreamMetrics(event);
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



  // Bottom bar content - using InfoBar component
  const bottomBar = <InfoBar />;

  return (
    <DashboardLayout
      onLogout={handleLogout}
      bottomBar={bottomBar}
    >
      <DashboardLayout.MainContent>
        <StreamsGrid
          streams={getStreamsArray()}
          loading={loading}
          error={error}
          onRefresh={fetchStreams}
          onDeleteStream={handleDeleteStream}
          onCreateStream={handleCreateStream}
          viewMode={viewMode}
          onViewModeChange={setViewMode}
        />
      </DashboardLayout.MainContent>
    </DashboardLayout>
  );
}