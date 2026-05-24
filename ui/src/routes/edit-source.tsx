import { useEffect } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { SourceForm } from '../components/sources/SourceForm';
import { DashboardLayout } from '../components/DashboardLayout';
import { Button } from '../components/Button';
import { InfoBar } from '../components/InfoBar';
import { useAuthStore } from '../hooks/useAuthStore';
import { useSourceStore } from '../hooks/useSourceStore';
import { formatUptime } from '../lib/formatUptime';

export default function EditSource() {
  const navigate = useNavigate();
  const { sourceId } = useParams<{ sourceId: string }>();
  const { logout } = useAuthStore();

  const sourceData = useSourceStore((s) =>
    sourceId ? s.sourcesById[sourceId] : undefined,
  );
  const lastUpdated = useSourceStore((s) => s.lastUpdated);
  const fetchSources = useSourceStore((s) => s.fetchSources);

  useEffect(() => {
    if (lastUpdated === null) {
      fetchSources();
    }
  }, [lastUpdated, fetchSources]);

  useEffect(() => {
    if (!sourceId || (lastUpdated !== null && !sourceData)) {
      navigate('/sources');
    }
  }, [sourceId, sourceData, lastUpdated, navigate]);

  if (!sourceId || lastUpdated === null || !sourceData) {
    return (
      <div className="flex items-center justify-center min-h-screen">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-gray-900 dark:border-white" />
      </div>
    );
  }

  const handleSuccess = async () => {
    navigate('/sources');
  };

  const handleCancel = () => {
    navigate('/sources');
  };

  const uptime = formatUptime(sourceData.started_at_us);

  return (
    <DashboardLayout onLogout={logout} bottomBar={<InfoBar />}>
      <DashboardLayout.MainContent>
        <div className="space-y-6">
          <div className="flex items-center justify-between">
            <div>
              <h1 className="text-2xl font-bold text-gray-900 dark:text-white">
                Edit Source: {sourceData.id}
              </h1>
              <p className="text-gray-600 dark:text-gray-300 mt-1">
                Update the configuration for this source.
                {uptime && (
                  <span className="ml-2 text-xs text-fg-subtle">
                    Uptime <span className="text-fg">{uptime}</span>
                  </span>
                )}
              </p>
            </div>
            <Button theme="light" onClick={handleCancel} size="SM" text="Back to Sources" />
          </div>

          <SourceForm
            key={sourceData.id}
            initialData={sourceData}
            onSuccess={handleSuccess}
            onCancel={handleCancel}
          />
        </div>
      </DashboardLayout.MainContent>
    </DashboardLayout>
  );
}
