import { useEffect, useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import { useShallow } from 'zustand/shallow';
import { useAuthStore } from '../hooks/useAuthStore';
import { useSourceStore, type SourceEntry } from '../hooks/useSourceStore';
import { useStreamStore } from '../hooks/useStreamStore';
import { useSourceStatusBridge } from '../hooks/useSourceStatusBridge';
import { DashboardLayout } from '../components/DashboardLayout';
import { Card } from '../components/Card';
import { SectionHeader } from '../components/primitives/SectionHeader';
import { SourceList } from '../components/sources/SourceList';

export default function Sources() {
  const navigate = useNavigate();
  const { logout } = useAuthStore();

  const sourceIds = useSourceStore((s) => s.sourceIds);
  const sourcesById = useSourceStore((s) => s.sourcesById);
  const { loading, error } = useSourceStore(
    useShallow((s) => ({ loading: s.loading, error: s.error })),
  );
  const fetchSources = useSourceStore((s) => s.fetchSources);
  const fetchStreams = useStreamStore((s) => s.fetchStreams);

  // Bridges SSE/stream data into the source catalog until B5/U3 land.
  useSourceStatusBridge();

  useEffect(() => {
    void fetchSources();
    void fetchStreams();
  }, [fetchSources, fetchStreams]);

  const sources = useMemo<SourceEntry[]>(
    () => sourceIds.map((id) => sourcesById[id]).filter((entry): entry is SourceEntry => !!entry),
    [sourceIds, sourcesById],
  );

  const handleLogout = () => {
    logout();
    navigate('/login');
  };

  return (
    <DashboardLayout onLogout={handleLogout}>
      <DashboardLayout.MainContent>
        <Card padding="lg">
          <SectionHeader
            title="Sources"
            description="Frame producers (V4L2 devices or test patterns) shared across composers and streams."
          />
          {error && (
            <div
              role="alert"
              className="mb-4 rounded border border-danger-soft bg-danger-soft text-danger-soft-fg px-3 py-2 text-sm"
            >
              {error}
            </div>
          )}
          {loading && sources.length === 0 ? (
            <p className="text-sm text-fg-muted">Loading sources…</p>
          ) : (
            <SourceList sources={sources} />
          )}
        </Card>
      </DashboardLayout.MainContent>
    </DashboardLayout>
  );
}
