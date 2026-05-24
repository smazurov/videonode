import { useEffect } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { StreamForm } from '../components/streams/StreamForm';
import { DashboardLayout } from '../components/DashboardLayout';
import { Button } from '../components/Button';
import { InfoBar } from '../components/InfoBar';
import { useAuthStore } from '../hooks/useAuthStore';
import { useStreamStore } from '../hooks/useStreamStore';

export default function EditStream() {
  const navigate = useNavigate();
  const { streamId } = useParams<{ streamId: string }>();
  const { logout } = useAuthStore();

  const streamData = useStreamStore((state) =>
    streamId ? state.streamsById[streamId] : undefined,
  );
  const lastUpdated = useStreamStore((state) => state.lastUpdated);
  const fetchStreams = useStreamStore((state) => state.fetchStreams);

  useEffect(() => {
    if (lastUpdated === null) {
      fetchStreams();
    }
  }, [lastUpdated, fetchStreams]);

  useEffect(() => {
    if (!streamId || (lastUpdated !== null && !streamData)) {
      navigate('/streams');
    }
  }, [streamId, streamData, lastUpdated, navigate]);

  if (!streamId || lastUpdated === null || !streamData) {
    return (
      <div className="flex items-center justify-center min-h-screen">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-gray-900 dark:border-white" />
      </div>
    );
  }

  const handleSuccess = async () => {
    navigate('/streams');
  };

  const handleCancel = () => {
    navigate('/streams');
  };

  return (
    <DashboardLayout onLogout={logout} bottomBar={<InfoBar />}>
      <DashboardLayout.MainContent>
        <div className="space-y-6">
          <div className="flex items-center justify-between">
            <div>
              <h1 className="text-2xl font-bold text-gray-900 dark:text-white">
                Edit Stream: {streamData.stream_id}
              </h1>
              <p className="text-gray-600 dark:text-gray-300 mt-1">
                Update the configuration for this stream
              </p>
            </div>
            <Button theme="light" onClick={handleCancel} size="SM" text="Back to Streams" />
          </div>

          <StreamForm
            key={streamData.stream_id}
            initialData={streamData}
            onSuccess={handleSuccess}
            onCancel={handleCancel}
          />
        </div>
      </DashboardLayout.MainContent>
    </DashboardLayout>
  );
}
