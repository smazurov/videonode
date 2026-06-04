import { useNavigate } from 'react-router-dom';
import { StreamForm } from '../components/streams/StreamForm';
import { DashboardLayout } from '../components/DashboardLayout';
import { Button } from '../components/Button';
import { InfoBar } from '../components/InfoBar';
import { useAuthStore } from '../hooks/useAuthStore';

export default function CreateStream() {
  const navigate = useNavigate();
  const logout = useAuthStore((s) => s.logout);

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
                Create New Stream
              </h1>
              <p className="text-gray-600 dark:text-gray-300 mt-1">
                Define an encoder + audio for an existing source or composer upstream.
              </p>
            </div>
            <Button theme="light" onClick={handleCancel} size="SM" text="Back to Streams" />
          </div>

          <StreamForm onSuccess={handleSuccess} onCancel={handleCancel} />
        </div>
      </DashboardLayout.MainContent>
    </DashboardLayout>
  );
}
