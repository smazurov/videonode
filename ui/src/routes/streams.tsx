import { useState, useEffect } from 'react';
import { useAuthStore } from '../hooks/useAuthStore';
import { useStreamStore } from '../hooks/useStreamStore';
import { useSSEManager } from '../hooks/useSSEManager';
import { DashboardLayout } from '../components/DashboardLayout';
import { InfoBar } from '../components/InfoBar';
import { StreamsGrid } from '../components/StreamsGrid';
import { StreamCreation } from '../components/StreamCreation';
import { StatsSidebar } from '../components/StatsSidebar';
import { Button } from '../components/Button';
import { 
  StreamRequestData,
  SSEStreamLifecycleEvent,
  SSEStreamMetricsEvent
} from '../lib/api';

export default function Streams() {
  const { logout } = useAuthStore();
  const { 
    loading, 
    error,
    fetchStreams,
    createStream,
    deleteStream,
    addStreamFromSSE,
    removeStreamFromSSE,
    updateStreamMetrics,
    getStreamsArray
  } = useStreamStore();
  
  // Local UI state
  const [viewMode, setViewMode] = useState<'grid' | 'tabs'>('grid');
  const [showCreateForm, setShowCreateForm] = useState(false);
  const [creating, setCreating] = useState(false);
  const [isStatsSidebarOpen, setIsStatsSidebarOpen] = useState(false);

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
      console.log('Received SSE stream metrics event:', event);
      updateStreamMetrics(event);
    }
  });

  // Load streams on mount
  useEffect(() => {
    fetchStreams();
  }, []);

  const handleCreateStream = async (streamData: StreamRequestData) => {
    setCreating(true);
    
    try {
      await createStream(streamData);
      setShowCreateForm(false);
      // SSE event will update the UI
    } catch (error) {
      console.error('Failed to create stream:', error);
      throw error; // Re-throw to let the form handle the error display
    } finally {
      setCreating(false);
    }
  };

  const handleDeleteStream = async (streamId: string) => {
    try {
      await deleteStream(streamId);
      // SSE event will update the UI
    } catch (error) {
      console.error('Failed to delete stream:', error);
      throw error;
    }
  };

  const handleLogout = () => {
    logout();
  };

  const handleToggleStats = () => {
    setIsStatsSidebarOpen(!isStatsSidebarOpen);
  };



  // Bottom bar content - using InfoBar component
  const bottomBar = <InfoBar />;

  return (
    <>
      <DashboardLayout
        onLogout={handleLogout}
        onToggleStats={handleToggleStats}
        bottomBar={bottomBar}
      >
      <DashboardLayout.MainContent>
        {showCreateForm ? (
          <div className="space-y-6">
            <div className="flex items-center justify-between">
              <div>
                <h1 className="text-2xl font-bold text-gray-900 dark:text-white">
                  Create New Stream
                </h1>
                <p className="text-gray-600 dark:text-gray-300 mt-1">
                  Configure a new video stream from your capture devices
                </p>
              </div>
              <div className="flex items-center space-x-2">
                <Button
                  theme="light"
                  onClick={() => setShowCreateForm(false)}
                  disabled={creating}
                  size="SM"
                  text="Back to Streams"
                />
              </div>
            </div>
            
            <StreamCreation
              onCreateStream={handleCreateStream}
              onCancel={() => setShowCreateForm(false)}
              isCreating={creating}
            />
          </div>
        ) : (
          <StreamsGrid
            streams={getStreamsArray()}
            loading={loading}
            error={error}
            onRefresh={fetchStreams}
            onDeleteStream={handleDeleteStream}
            onCreateStream={() => setShowCreateForm(true)}
            viewMode={viewMode}
            onViewModeChange={setViewMode}
          />
        )}
      </DashboardLayout.MainContent>
    </DashboardLayout>
    
    {/* Stats Sidebar */}
    <StatsSidebar 
      isOpen={isStatsSidebarOpen} 
      onToggle={handleToggleStats}
    />
    </>
  );
}