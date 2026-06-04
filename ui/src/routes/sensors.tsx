import { useEffect, useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import { useShallow } from 'zustand/shallow';

import { useAuthStore } from '../hooks/useAuthStore';
import { useSensorStore } from '../hooks/useSensorStore';
import type { Sensor } from '../hooks/useSensorStore';
import { DashboardLayout } from '../components/DashboardLayout';
import { InfoBar } from '../components/InfoBar';
import { Button } from '../components/Button';
import { Card } from '../components/Card';
import { Spinner } from '../components/Spinner';
import { SectionHeader } from '../components/primitives/SectionHeader';
import { SensorList } from '../components/sensors/SensorList';

export default function Sensors() {
  const navigate = useNavigate();
  const logout = useAuthStore((s) => s.logout);

  const sensorIds = useSensorStore((s) => s.sensorIds);
  const sensorsById = useSensorStore((s) => s.sensorsById);
  const { loading, error, lastUpdated } = useSensorStore(
    useShallow((s) => ({ loading: s.loading, error: s.error, lastUpdated: s.lastUpdated })),
  );
  const fetchSensors = useSensorStore((s) => s.fetchSensors);

  useEffect(() => {
    if (lastUpdated === null) void fetchSensors();
  }, [lastUpdated, fetchSensors]);

  const sensors = useMemo<Sensor[]>(
    () => sensorIds.map((id) => sensorsById[id]).filter((s): s is Sensor => !!s),
    [sensorIds, sensorsById],
  );

  const handleLogout = () => {
    logout();
    navigate('/login');
  };

  return (
    <DashboardLayout onLogout={handleLogout} bottomBar={<InfoBar />}>
      <DashboardLayout.MainContent>
        <Card padding="lg" className="space-y-6">
          <SectionHeader
            title="Sensors"
            description="AI perception — each sensor observes a source or composer, runs a detector, and emits findings. A composer input's AI auto-crop just picks a sensor."
            actions={
              <Button
                theme="primary"
                size="SM"
                text="New sensor"
                onClick={() => navigate('/sensors/new')}
              />
            }
          />
          {error && (
            <div className="p-3 border border-danger rounded-md bg-danger-soft text-sm text-danger-soft-fg">
              {error}
            </div>
          )}
          {loading && lastUpdated === null ? (
            <div className="flex items-center justify-center py-12">
              <Spinner />
            </div>
          ) : (
            <SensorList sensors={sensors} />
          )}
        </Card>
      </DashboardLayout.MainContent>
    </DashboardLayout>
  );
}
