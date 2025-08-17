import { useState, useEffect } from 'react';
import { useAuthStore } from '../hooks/useAuthStore';
import { DashboardLayout } from '../components/DashboardLayout';
import { InfoBar } from '../components/InfoBar';
import { StreamsGrid } from '../components/StreamsGrid';
import { StreamCreation } from '../components/StreamCreation';
import { StatsSidebar } from '../components/StatsSidebar';
import { Button } from '../components/Button';
import { 
  StreamData, 
  StreamRequestData, 
  getStreams, 
  createStream, 
  deleteStream
} from '../lib/api';
import { ApiError } from '../lib/api';

export default function Streams() {
  const { logout } = useAuthStore();
  
  // State management
  const [streams, setStreams] = useState<StreamData[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [viewMode, setViewMode] = useState<'grid' | 'tabs'>('grid');
  const [showCreateForm, setShowCreateForm] = useState(false);
  const [creating, setCreating] = useState(false);
  const [isStatsSidebarOpen, setIsStatsSidebarOpen] = useState(false);

  // Load streams on mount
  useEffect(() => {
    loadStreams();
  }, []);

  const loadStreams = async () => {
    setLoading(true);
    setError(null);
    
    try {
      const streamData = await getStreams();
      setStreams(streamData.streams);
    } catch (error) {
      console.error('Failed to load streams:', error);
      if (error instanceof ApiError) {
        setError(error.message);
      } else {
        setError('Failed to load streams');
      }
    } finally {
      setLoading(false);
    }
  };

  const handleCreateStream = async (streamData: StreamRequestData) => {
    setCreating(true);
    
    try {
      const newStream = await createStream(streamData);
      setStreams(prev => [...prev, newStream]);
      setShowCreateForm(false);
      
      // Refresh the streams list to get the latest data
      setTimeout(loadStreams, 1000);
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
      setStreams(prev => prev.filter(s => s.stream_id !== streamId));
    } catch (error) {
      console.error('Failed to delete stream:', error);
      throw error;
    }
  };

  const handleCaptureStream = async (streamId: string) => {
    try {
      // Find the stream to get device info
      const stream = streams.find(s => s.stream_id === streamId);
      if (!stream) {
        throw new Error('Stream not found');
      }

      // We need to get the device path from the stream's device_id
      // For now, we'll just log this as a placeholder
      console.log('Capturing from stream:', streamId, 'device:', stream.device_id);
      
      // In a real implementation, you'd need to:
      // 1. Get device info from device_id to find the device_path
      // 2. Call captureFromDevice with the device_path
      // 3. Handle the returned screenshot data
      
      // Placeholder implementation:
      alert(`Capture functionality not fully implemented yet for stream: ${streamId}`);
    } catch (error) {
      console.error('Failed to capture from stream:', error);
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
            streams={streams}
            loading={loading}
            error={error}
            onRefresh={loadStreams}
            onDeleteStream={handleDeleteStream}
            onCaptureStream={handleCaptureStream}
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