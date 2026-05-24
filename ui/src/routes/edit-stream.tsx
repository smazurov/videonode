// Pending U13: new sectioned StreamForm
// (Identity / Encoder / Audio / Publish). The old monolithic form was
// deleted by U14; this stub keeps the build green until U13 lands.
import { useNavigate, useParams } from 'react-router-dom';
import { DashboardLayout } from '../components/DashboardLayout';
import { Button } from '../components/Button';
import { InfoBar } from '../components/InfoBar';
import { useAuthStore } from '../hooks/useAuthStore';

export default function EditStream() {
  const navigate = useNavigate();
  const { streamId } = useParams<{ streamId: string }>();
  const { logout } = useAuthStore();

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
                Edit Stream{streamId ? `: ${streamId}` : ''}
              </h1>
              <p className="text-gray-600 dark:text-gray-300 mt-1">
                The stream editor is being rebuilt (U13).
              </p>
            </div>
            <Button theme="light" onClick={handleCancel} size="SM" text="Back to Streams" />
          </div>

          <div className="p-8 text-sm text-fg-muted border border-border rounded-md bg-surface-muted">
            Stream editing UI is pending the U13 refactor.
          </div>
        </div>
      </DashboardLayout.MainContent>
    </DashboardLayout>
  );
}
