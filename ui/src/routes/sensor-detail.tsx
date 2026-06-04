import { useCallback, useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { useShallow } from 'zustand/shallow';
import toast from 'react-hot-toast';
import { ArrowPathIcon } from '@heroicons/react/24/outline';

import { useAuthStore } from '../hooks/useAuthStore';
import { useSensorStore } from '../hooks/useSensorStore';
import { DashboardLayout } from '../components/DashboardLayout';
import { InfoBar } from '../components/InfoBar';
import { Button } from '../components/Button';
import { Card } from '../components/Card';
import { Spinner } from '../components/Spinner';
import { StatusPill } from '../components/primitives/StatusPill';
import { SectionHeader } from '../components/primitives/SectionHeader';
import { SensorForm } from '../components/sensors/SensorForm';
import { SensorFindingsFeed } from '../components/sensors/SensorFindingsFeed';
import { EntityLogsPanel } from '../components/logs/EntityLogsPanel';
import { api } from '../lib/api';
import { poolStateToPill, isRestartable } from '../lib/pool-status';

export default function SensorDetail() {
  const navigate = useNavigate();
  const { sensorId } = useParams<{ sensorId: string }>();
  const logout = useAuthStore((s) => s.logout);

  const sensor = useSensorStore((s) => (sensorId ? s.sensorsById[sensorId] : undefined));
  const findings = useSensorStore((s) => (sensorId ? s.recentFindingsById[sensorId] : undefined));
  const { loading, error, lastUpdated } = useSensorStore(
    useShallow((s) => ({ loading: s.loading, error: s.error, lastUpdated: s.lastUpdated })),
  );
  const fetchSensors = useSensorStore((s) => s.fetchSensors);
  const deleteSensor = useSensorStore((s) => s.deleteSensor);
  const [editing, setEditing] = useState(false);

  useEffect(() => {
    if (lastUpdated === null) void fetchSensors();
  }, [lastUpdated, fetchSensors]);

  const handleLogout = () => {
    logout();
    navigate('/login');
  };

  const handleRestart = useCallback(async () => {
    if (!sensorId) return;
    try {
      const { error: e } = await api.POST('/api/processes/{id}/restart', {
        params: { path: { id: `sensor:${sensorId}` } },
      });
      if (e) throw new Error(e.detail ?? 'Failed to restart sensor');
      toast.success(`Restart requested for '${sensorId}'`);
      void fetchSensors();
    } catch (error_) {
      toast.error(error_ instanceof Error ? error_.message : 'Failed to restart sensor');
    }
  }, [sensorId, fetchSensors]);

  const handleDelete = useCallback(async () => {
    if (!sensorId) return;
    try {
      await deleteSensor(sensorId);
      toast.success(`Sensor ${sensorId} deleted`);
      navigate('/sensors');
    } catch (error_) {
      toast.error(error_ instanceof Error ? error_.message : 'Failed to delete sensor');
    }
  }, [sensorId, deleteSensor, navigate]);

  if (lastUpdated === null && loading) {
    return (
      <DashboardLayout onLogout={handleLogout} bottomBar={<InfoBar />}>
        <DashboardLayout.MainContent>
          <div className="flex items-center justify-center py-12">
            <Spinner />
          </div>
        </DashboardLayout.MainContent>
      </DashboardLayout>
    );
  }

  if (!sensorId || !sensor) {
    return (
      <DashboardLayout onLogout={handleLogout} bottomBar={<InfoBar />}>
        <DashboardLayout.MainContent>
          <Card padding="lg">
            <p className="text-fg-muted">
              Sensor not found.{' '}
              <Link to="/sensors" className="underline">
                Back to sensors
              </Link>
            </p>
          </Card>
        </DashboardLayout.MainContent>
      </DashboardLayout>
    );
  }

  return (
    <DashboardLayout onLogout={handleLogout} bottomBar={<InfoBar />}>
      <DashboardLayout.MainContent>
        <div className="space-y-6">
          <div className="flex items-center justify-between">
            <SectionHeader title={sensor.id} description={`Observes ${sensor.source}`} />
            <div className="flex gap-2">
              <Button theme="light" size="SM" text="Back" onClick={() => navigate('/sensors')} />
              <Button
                theme="light"
                size="SM"
                text="Restart"
                LeadingIcon={ArrowPathIcon}
                disabled={!isRestartable(sensor.status)}
                onClick={handleRestart}
              />
              <Button
                theme="primary"
                size="SM"
                text={editing ? 'Close editor' : 'Edit'}
                onClick={() => setEditing((v) => !v)}
              />
              <Button theme="danger" size="SM" text="Delete" onClick={handleDelete} />
            </div>
          </div>

          {error && (
            <div className="p-3 border border-danger rounded-md bg-danger-soft text-sm text-danger-soft-fg">
              {error}
            </div>
          )}

          {editing ? (
            <SensorForm
              initialData={sensor}
              onSuccess={() => setEditing(false)}
              onCancel={() => setEditing(false)}
            />
          ) : (
            <Card padding="lg" className="space-y-4">
              <div className="flex items-center justify-between">
                <h3 className="text-sm font-semibold text-fg">Configuration</h3>
                <StatusPill status={poolStateToPill(sensor.status)} />
              </div>
              <dl className="grid grid-cols-2 gap-x-6 gap-y-2 text-sm sm:grid-cols-3">
                <Field label="Observes" value={sensor.source} mono />
                <Field label="Mode" value={sensor.mode ?? 'auto'} />
                <Field label="Margin" value={String(sensor.margin ?? 0.1)} />
                <Field label="Min confidence" value={String(sensor.min_confidence ?? 0.8)} />
                <Field label="Re-detect (ms)" value={String(sensor.tick_ms ?? 0)} />
                <Field label="Model" value={sensor.model_id || '(default)'} />
                <Field label="Detector" value={sensor.detector || '(daemon default)'} mono wide />
              </dl>
              <div>
                <h4 className="mb-1 text-xs font-semibold uppercase tracking-wide text-fg-subtle">
                  Bound to
                </h4>
                {sensor.bindings?.length ? (
                  <ul className="space-y-1 text-xs font-mono text-fg-muted">
                    {sensor.bindings.map((b) => (
                      <li key={`${b.id}-${b.input}`}>
                        {b.kind}:{b.id} → {b.input}
                      </li>
                    ))}
                  </ul>
                ) : (
                  <p className="text-xs text-fg-subtle">
                    Unattached — running and emitting findings, but no composer input drives a
                    crop from it yet. Attach it from a composer input&apos;s AI auto-crop effect.
                  </p>
                )}
              </div>
            </Card>
          )}

          <Card padding="lg" className="space-y-3">
            <h3 className="text-sm font-semibold text-fg">Live findings</h3>
            <SensorFindingsFeed findings={findings ?? []} />
          </Card>

          <EntityLogsPanel
            filter={{ key: 'sensor_id', id: sensor.id }}
            description={`Live logs for sensor ${sensor.id}.`}
          />
        </div>
      </DashboardLayout.MainContent>
    </DashboardLayout>
  );
}

function Field({
  label,
  value,
  mono,
  wide,
}: Readonly<{ label: string; value: string; mono?: boolean; wide?: boolean }>) {
  return (
    <div className={wide ? 'col-span-2 sm:col-span-3' : undefined}>
      <dt className="text-xs text-fg-subtle">{label}</dt>
      <dd className={mono ? 'font-mono text-fg' : 'text-fg'}>{value}</dd>
    </div>
  );
}
