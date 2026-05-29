import { useNavigate } from 'react-router-dom';
import { SourceForm } from '../components/sources/SourceForm';
import { DashboardLayout } from '../components/DashboardLayout';
import { Button } from '../components/Button';
import { InfoBar } from '../components/InfoBar';
import { useAuthStore } from '../hooks/useAuthStore';

export default function CreateSource() {
  const navigate = useNavigate();
  const logout = useAuthStore((s) => s.logout);

  const handleSuccess = async () => {
    navigate('/sources');
  };

  const handleCancel = () => {
    navigate('/sources');
  };

  return (
    <DashboardLayout onLogout={logout} bottomBar={<InfoBar />}>
      <DashboardLayout.MainContent>
        <div className="space-y-6">
          <div className="flex items-center justify-between">
            <div>
              <h1 className="text-2xl font-bold text-gray-900 dark:text-white">
                Create New Source
              </h1>
              <p className="text-gray-600 dark:text-gray-300 mt-1">
                Add a capture device, or a test-pattern producer for
                hardware-free pipeline runs.
              </p>
            </div>
            <Button theme="light" onClick={handleCancel} size="SM" text="Back to Sources" />
          </div>

          <SourceForm onSuccess={handleSuccess} onCancel={handleCancel} />
        </div>
      </DashboardLayout.MainContent>
    </DashboardLayout>
  );
}
