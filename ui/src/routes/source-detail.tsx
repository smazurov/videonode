import { useEffect, useMemo } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { useAuthStore } from '../hooks/useAuthStore';
import { useSourceStore } from '../hooks/useSourceStore';
import { useStreamStore } from '../hooks/useStreamStore';
import { useSourceStatusBridge } from '../hooks/useSourceStatusBridge';
import { DashboardLayout } from '../components/DashboardLayout';
import { EntityDetailLayout, type EntityListItem } from '../components/EntityDetailLayout';
import { Badge } from '../components/Badge';
import { EmptyState } from '../components/primitives/EmptyState';
import { SourceLivePreview } from '../components/sources/SourceLivePreview';
import { SourceStatusPanel } from '../components/sources/SourceStatusPanel';
import { SourceFormatPanel } from '../components/sources/SourceFormatPanel';
import { SourceConsumersPanel } from '../components/sources/SourceConsumersPanel';

export default function SourceDetail() {
  const { sourceId = '' } = useParams<{ sourceId: string }>();
  const navigate = useNavigate();
  const { logout } = useAuthStore();

  const sourceIds = useSourceStore((s) => s.sourceIds);
  const sourcesById = useSourceStore((s) => s.sourcesById);
  const fetchSources = useSourceStore((s) => s.fetchSources);
  const fetchStreams = useStreamStore((s) => s.fetchStreams);

  useSourceStatusBridge();

  useEffect(() => {
    void fetchSources();
    void fetchStreams();
  }, [fetchSources, fetchStreams]);

  const source = sourcesById[sourceId];

  const siblings = useMemo<EntityListItem[]>(
    () =>
      sourceIds.map((id) => {
        const entry = sourcesById[id]!;
        const sub = entry.test_mode ? 'test pattern' : entry.device ?? '';
        return {
          id,
          label: id,
          to: `/sources/${id}`,
          ...(sub ? { sublabel: sub } : {}),
          active: id === sourceId,
        };
      }),
    [sourceIds, sourcesById, sourceId],
  );

  const handleLogout = () => {
    logout();
    navigate('/login');
  };

  const titleNode = (
    <span className="inline-flex items-center gap-2">
      <span>{sourceId || 'Source'}</span>
      {source?.test_mode && <Badge tone="info" size="sm">test pattern</Badge>}
    </span>
  );

  return (
    <DashboardLayout onLogout={handleLogout}>
      <DashboardLayout.MainContent>
        <EntityDetailLayout
          title={titleNode}
          breadcrumbs={[
            { label: 'Sources', to: '/sources' },
            { label: sourceId || 'Detail' },
          ]}
          siblings={siblings}
          siblingsTitle="Sources"
          siblingsEmpty="No sources"
        >
          {source ? (
            <>
              <SourceLivePreview sourceId={sourceId} />
              <div className="grid grid-cols-1 xl:grid-cols-2 gap-4">
                <SourceStatusPanel source={source} />
                <SourceFormatPanel source={source} />
              </div>
              <SourceConsumersPanel consumers={source.consumers} />
            </>
          ) : (
            <EmptyState
              title="Source not found"
              description={`No source with id "${sourceId}" is currently known. It may have been deleted, or the daemon hasn't pushed a status frame yet.`}
            />
          )}
        </EntityDetailLayout>
      </DashboardLayout.MainContent>
    </DashboardLayout>
  );
}
