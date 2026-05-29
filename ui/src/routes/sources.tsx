import { useEffect, useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import { useShallow } from 'zustand/shallow';

import { useAuthStore } from '../hooks/useAuthStore';
import { useSourceStore, type SourceEntry } from '../hooks/useSourceStore';
import { DashboardLayout } from '../components/DashboardLayout';
import { InfoBar } from '../components/InfoBar';
import { Button } from '../components/Button';
import { Card } from '../components/Card';
import { Spinner } from '../components/Spinner';
import { SectionHeader } from '../components/primitives/SectionHeader';
import { SourceList } from '../components/sources/SourceList';

export default function Sources() {
  const navigate = useNavigate();
  const logout = useAuthStore((s) => s.logout);

  const sourceIds = useSourceStore((s) => s.sourceIds);
  const sourcesById = useSourceStore((s) => s.sourcesById);
  const { loading, error, lastUpdated } = useSourceStore(
    useShallow((s) => ({ loading: s.loading, error: s.error, lastUpdated: s.lastUpdated })),
  );
  const fetchSources = useSourceStore((s) => s.fetchSources);

  useEffect(() => {
    if (lastUpdated === null) void fetchSources();
  }, [lastUpdated, fetchSources]);

  const sources = useMemo<SourceEntry[]>(
    () => sourceIds.map((id) => sourcesById[id]).filter((s): s is SourceEntry => !!s),
    [sourceIds, sourcesById],
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
            title="Sources"
            description="Frame producers — V4L2 devices or test patterns. Sources stay warm whenever configured."
            actions={
              <Button
                theme="primary"
                size="SM"
                text="New source"
                onClick={() => navigate('/sources/new')}
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
            <SourceList sources={sources} />
          )}
        </Card>
      </DashboardLayout.MainContent>
    </DashboardLayout>
  );
}
