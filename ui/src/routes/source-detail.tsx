import { useEffect } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { useShallow } from 'zustand/shallow';

import { useAuthStore } from '../hooks/useAuthStore';
import { useSourceStore } from '../hooks/useSourceStore';
import { DashboardLayout } from '../components/DashboardLayout';
import { InfoBar } from '../components/InfoBar';
import { Button } from '../components/Button';
import { Card } from '../components/Card';
import { Spinner } from '../components/Spinner';
import { SectionHeader } from '../components/primitives/SectionHeader';
import { SourceFormatPanel } from '../components/sources/SourceFormatPanel';
import { SourceStatusPanel } from '../components/sources/SourceStatusPanel';
import { SourceConsumersPanel } from '../components/sources/SourceConsumersPanel';

export default function SourceDetail() {
  const navigate = useNavigate();
  const { sourceId } = useParams<{ sourceId: string }>();
  const { logout } = useAuthStore();

  const source = useSourceStore((s) => (sourceId ? s.sourcesById[sourceId] : undefined));
  const { loading, error, lastUpdated } = useSourceStore(
    useShallow((s) => ({ loading: s.loading, error: s.error, lastUpdated: s.lastUpdated })),
  );
  const fetchSources = useSourceStore((s) => s.fetchSources);

  useEffect(() => {
    if (lastUpdated === null) void fetchSources();
  }, [lastUpdated, fetchSources]);

  const handleLogout = () => {
    logout();
    navigate('/login');
  };

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

  if (!sourceId || !source) {
    return (
      <DashboardLayout onLogout={handleLogout} bottomBar={<InfoBar />}>
        <DashboardLayout.MainContent>
          <Card padding="lg">
            <p className="text-fg-muted">
              Source not found.{' '}
              <Link to="/sources" className="underline">
                Back to sources
              </Link>
            </p>
          </Card>
        </DashboardLayout.MainContent>
      </DashboardLayout>
    );
  }

  let description = '';
  if (source.test_mode) {
    description = 'Test pattern source.';
  } else if (source.device) {
    description = `Device: ${source.device}`;
  }

  return (
    <DashboardLayout onLogout={handleLogout} bottomBar={<InfoBar />}>
      <DashboardLayout.MainContent>
        <div className="space-y-6">
          <div className="flex items-center justify-between">
            <SectionHeader title={source.id} description={description} />
            <div className="flex gap-2">
              <Button theme="light" size="SM" text="Back" onClick={() => navigate('/sources')} />
              <Button
                theme="primary"
                size="SM"
                text="Edit"
                onClick={() => navigate(`/sources/${source.id}/edit`)}
              />
            </div>
          </div>

          {error && (
            <div className="p-3 border border-danger rounded-md bg-danger-soft text-sm text-danger-soft-fg">
              {error}
            </div>
          )}

          <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
            <SourceStatusPanel source={source} />
            <SourceFormatPanel source={source} />
          </div>
          <SourceConsumersPanel consumers={source.consumers ?? []} />
        </div>
      </DashboardLayout.MainContent>
    </DashboardLayout>
  );
}
