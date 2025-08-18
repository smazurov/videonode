import { useNavigate } from 'react-router-dom';
import { StreamCreation } from '../components/StreamCreation';
import { DashboardLayout } from '../components/DashboardLayout';
import { Button } from '../components/Button';
import { InfoBar } from '../components/InfoBar';
import { useAuthStore } from '../hooks/useAuthStore';

export default function CreateStream() {
  const navigate = useNavigate();
  const { logout } = useAuthStore();

  const handleCreateStream = async () => {
    // Stream is already created by the useStreamCreation hook
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
                Create New Stream
              </h1>
              <p className="text-gray-600 dark:text-gray-300 mt-1">
                Configure a new video stream from your capture devices
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
            onCreateStream={handleCreateStream}
            onCancel={handleCancel}
          />
        </div>
      </DashboardLayout.MainContent>
    </DashboardLayout>
  );
}