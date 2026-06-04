import { useNavigate } from 'react-router-dom';

import { SensorForm } from '../components/sensors/SensorForm';
import { DashboardLayout } from '../components/DashboardLayout';
import { Button } from '../components/Button';
import { InfoBar } from '../components/InfoBar';
import { useAuthStore } from '../hooks/useAuthStore';

export default function CreateSensor() {
  const navigate = useNavigate();
  const logout = useAuthStore((s) => s.logout);

  const handleSuccess = async () => {
    navigate('/sensors');
  };
  const handleCancel = () => {
    navigate('/sensors');
  };

  return (
    <DashboardLayout onLogout={logout} bottomBar={<InfoBar />}>
      <DashboardLayout.MainContent>
        <div className="space-y-6">
          <div className="flex items-center justify-between">
            <div>
              <h1 className="text-2xl font-bold text-fg">Create New Sensor</h1>
              <p className="text-fg-muted mt-1">
                Observe a source or composer with a detector. The sensor runs and emits
                findings on its own; a composer input can pick it for AI auto-crop.
              </p>
            </div>
            <Button theme="light" onClick={handleCancel} size="SM" text="Back to Sensors" />
          </div>

          <SensorForm onSuccess={handleSuccess} onCancel={handleCancel} />
        </div>
      </DashboardLayout.MainContent>
    </DashboardLayout>
  );
}
