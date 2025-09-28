import { useNavigate, useParams } from 'react-router-dom';
import { StreamCreation } from '../components/StreamCreation';
import { DashboardLayout } from '../components/DashboardLayout';
import { Button } from '../components/Button';
import { InfoBar } from '../components/InfoBar';
import { useAuthStore } from '../hooks/useAuthStore';
import { useStreamStore } from '../hooks/useStreamStore';

export default function EditStream() {
  const navigate = useNavigate();
  const { streamId } = useParams<{ streamId: string }>();
  const { logout } = useAuthStore();
  const { getStreamById } = useStreamStore();

  if (!streamId) {
    navigate('/streams');
    return null;
  }

  const streamData = getStreamById(streamId);
  if (!streamData) {
    navigate('/streams');
    return null;
  }

  const handleUpdateStream = async () => {
    // Stream is already updated by the useStreamCreation hook
    // Just navigate back to streams list
    navigate('/streams');
  };

  const handleCancel = () => {
    navigate('/streams');
  };

  const handleLogout = () => {
    logout();
  };

  const bottomBar = <InfoBar />;

  return (
    <DashboardLayout
      onLogout={handleLogout}
      bottomBar={bottomBar}
    >
      <DashboardLayout.MainContent>
        <div className="space-y-6">
          <div className="flex items-center justify-between">
            <div>
              <h1 className="text-2xl font-bold text-gray-900 dark:text-white">
                Edit Stream: {streamData.stream_id}
              </h1>
              <p className="text-gray-600 dark:text-gray-300 mt-1">
                Update the configuration for this video stream
              </p>
            </div>
            <div className="flex items-center space-x-2">
              <Button
                theme="light"
                onClick={handleCancel}
                size="SM"
                text="Back to Streams"
              />
            </div>
          </div>
          
          <StreamCreation
            initialData={streamData}
            onCreateStream={handleUpdateStream}
            onCancel={handleCancel}
          />
        </div>
      </DashboardLayout.MainContent>
    </DashboardLayout>
  );
}