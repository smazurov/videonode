import { useCallback, useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { useShallow } from 'zustand/shallow';
import toast from 'react-hot-toast';
import { ArrowPathIcon } from '@heroicons/react/24/outline';

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
import { SourceLivePreview } from '../components/sources/SourceLivePreview';
import { SourceDeleteDialog } from '../components/sources/SourceDeleteDialog';
import { EntityLogsPanel } from '../components/logs/EntityLogsPanel';
import { api } from '../lib/api';
import { isRestartable } from '../lib/pool-status';

export default function SourceDetail() {
  const navigate = useNavigate();
  const { sourceId } = useParams<{ sourceId: string }>();
  const logout = useAuthStore((s) => s.logout);

  const source = useSourceStore((s) => (sourceId ? s.sourcesById[sourceId] : undefined));
  const { loading, error, lastUpdated } = useSourceStore(
    useShallow((s) => ({ loading: s.loading, error: s.error, lastUpdated: s.lastUpdated })),
  );
  const fetchSources = useSourceStore((s) => s.fetchSources);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [previewVisible, setPreviewVisible] = useState(false);

  useEffect(() => {
    if (lastUpdated === null) void fetchSources();
  }, [lastUpdated, fetchSources]);

  const handleLogout = () => {
    logout();
    navigate('/login');
  };

  const handleRestart = useCallback(async () => {
    if (!sourceId) return;
    try {
      const { error } = await api.POST('/api/processes/{id}/restart', {
        params: { path: { id: `source:${sourceId}` } },
      });
      if (error) throw new Error(error.detail ?? 'Failed to restart source');
      toast.success(`Restart requested for '${sourceId}'`);
      void fetchSources();
    } catch (error_) {
      console.error('Failed to restart source:', error_);
      toast.error('Failed to restart source');
    }
  }, [sourceId, fetchSources]);

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
              <Button
                theme="light"
                size="SM"
                text={previewVisible ? 'Hide preview' : 'Show preview'}
                onClick={() => setPreviewVisible((v) => !v)}
              />
              <Button theme="light" size="SM" text="Back" onClick={() => navigate('/sources')} />
              <Button
                theme="light"
                size="SM"
                text="Restart"
                LeadingIcon={ArrowPathIcon}
                disabled={!isRestartable(source.status)}
                onClick={handleRestart}
              />
              <Button
                theme="primary"
                size="SM"
                text="Edit"
                onClick={() => navigate(`/sources/${source.id}/edit`)}
              />
              <Button
                theme="danger"
                size="SM"
                text="Delete"
                onClick={() => setDeleteOpen(true)}
              />
            </div>
          </div>

          {error && (
            <div className="p-3 border border-danger rounded-md bg-danger-soft text-sm text-danger-soft-fg">
              {error}
            </div>
          )}

          {previewVisible ? (
            <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
              <SourceLivePreview
                sourceId={source.id}
                visible={previewVisible}
                onToggle={() => setPreviewVisible(false)}
              />
              <div className="space-y-4">
                <SourceStatusPanel source={source} />
                <SourceFormatPanel source={source} />
              </div>
            </div>
          ) : (
            <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
              <SourceStatusPanel source={source} />
              <SourceFormatPanel source={source} />
            </div>
          )}
          <SourceConsumersPanel consumers={source.consumers ?? []} />
          <EntityLogsPanel
            filter={{ key: 'source_id', id: source.id }}
            description={`Live logs for source ${source.id}.`}
          />
        </div>
      </DashboardLayout.MainContent>
      <SourceDeleteDialog
        sourceId={source.id}
        consumers={source.consumers ?? []}
        open={deleteOpen}
        onClose={() => setDeleteOpen(false)}
        onDeleted={() => navigate('/sources')}
      />
    </DashboardLayout>
  );
}
