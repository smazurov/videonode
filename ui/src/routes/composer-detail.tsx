import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { useShallow } from 'zustand/shallow';
import toast from 'react-hot-toast';
import {
  ArrowPathIcon,
  DocumentArrowDownIcon,
  DocumentArrowUpIcon,
} from '@heroicons/react/24/outline';

import { useAuthStore } from '../hooks/useAuthStore';
import { useComposerStore } from '../hooks/useComposerStore';
import { useStreamStore } from '../hooks/useStreamStore';
import { DashboardLayout } from '../components/DashboardLayout';
import { InfoBar } from '../components/InfoBar';
import { Button } from '../components/Button';
import { Card } from '../components/Card';
import { Spinner } from '../components/Spinner';
import { SectionHeader } from '../components/primitives/SectionHeader';
import { ComposerCanvasEditor } from '../components/composers/ComposerCanvasEditor';
import { ComposerInputsPanel } from '../components/composers/ComposerInputsPanel';
import { ComposerLayoutPanel } from '../components/composers/ComposerLayoutPanel';
import { ComposerConsumersPanel } from '../components/composers/ComposerConsumersPanel';
import { ComposerLivePreview } from '../components/composers/ComposerLivePreview';
import { ComposerDeleteDialog } from '../components/composers/ComposerDeleteDialog';
import { ComposerExportDialog } from '../components/composers/ComposerExportDialog';
import { ComposerImportDialog } from '../components/composers/ComposerImportDialog';
import { EntityLogsPanel } from '../components/logs/EntityLogsPanel';
import type { ComposerData } from '../lib/composer-types';
import { canvasFpsOrDefault } from '../lib/composer-types';
import { api } from '../lib/api';
import { isRestartable } from '../lib/pool-status';

export default function ComposerDetail() {
  const navigate = useNavigate();
  const { composerId } = useParams<{ composerId: string }>();
  const logout = useAuthStore((s) => s.logout);

  const composer = useComposerStore((s) =>
    composerId ? s.composersById[composerId] : undefined,
  );
  const { loading, error, lastUpdated } = useComposerStore(
    useShallow((s) => ({ loading: s.loading, error: s.error, lastUpdated: s.lastUpdated })),
  );
  const fetchComposers = useComposerStore((s) => s.fetchComposers);
  const importComposerTomlInto = useComposerStore((s) => s.importComposerTomlInto);

  const streamIds = useStreamStore((s) => s.streamIds);
  const streamsById = useStreamStore((s) => s.streamsById);
  const fetchStreams = useStreamStore((s) => s.fetchStreams);
  const streamsLastUpdated = useStreamStore((s) => s.lastUpdated);

  useEffect(() => {
    if (lastUpdated === null) void fetchComposers();
  }, [lastUpdated, fetchComposers]);
  useEffect(() => {
    if (streamsLastUpdated === null) void fetchStreams();
  }, [streamsLastUpdated, fetchStreams]);

  // Bridge: API returns `id`; ComposerData uses `composer_id`.
  const data = useMemo<ComposerData | undefined>(() => {
    if (!composer) return undefined;
    const { id, inputs, layout, ...rest } = composer;
    return { composer_id: id, inputs: inputs ?? [], layout: layout ?? [], ...rest } as ComposerData;
  }, [composer]);

  const streamRefs = useMemo(
    () =>
      streamIds
        .map((id) => streamsById[id])
        .filter((s): s is NonNullable<typeof s> => Boolean(s))
        .map((s) => ({ stream_id: s.stream_id, upstream: s.upstream })),
    [streamIds, streamsById],
  );

  const [deleteOpen, setDeleteOpen] = useState(false);
  const [exportOpen, setExportOpen] = useState(false);
  const [importOpen, setImportOpen] = useState(false);
  const [previewVisible, setPreviewVisible] = useState(false);

  const handleLogout = () => {
    logout();
    navigate('/login');
  };

  const handleRestart = useCallback(async () => {
    if (!composerId) return;
    try {
      const { error } = await api.POST('/api/processes/{id}/restart', {
        params: { path: { id: `composer:${composerId}` } },
      });
      if (error) throw new Error(error.detail ?? 'Failed to restart composer');
      toast.success(`Restart requested for '${composerId}'`);
      void fetchComposers();
    } catch (error_) {
      console.error('Failed to restart composer:', error_);
      toast.error('Failed to restart composer');
    }
  }, [composerId, fetchComposers]);

  const handleImport = useCallback(
    async (toml: string) => {
      if (!composerId) return;
      await importComposerTomlInto(composerId, toml);
      toast.success(`Imported TOML into '${composerId}'`);
    },
    [composerId, importComposerTomlInto],
  );

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

  if (!composerId || !data) {
    return (
      <DashboardLayout onLogout={handleLogout} bottomBar={<InfoBar />}>
        <DashboardLayout.MainContent>
          <Card padding="lg">
            <p className="text-fg-muted">
              Composer not found.{' '}
              <Link to="/composers" className="underline">
                Back to composers
              </Link>
            </p>
          </Card>
        </DashboardLayout.MainContent>
      </DashboardLayout>
    );
  }

  const fps = canvasFpsOrDefault(data.canvas);

  return (
    <DashboardLayout onLogout={handleLogout} bottomBar={<InfoBar />}>
      <DashboardLayout.MainContent>
        <div className="space-y-6">
          <div className="flex items-center justify-between">
            <SectionHeader
              title={data.composer_id}
              description={`Canvas ${data.canvas.w}×${data.canvas.h} @ ${fps} fps, ${data.inputs.length} input${data.inputs.length === 1 ? '' : 's'}`}
            />
            <div className="flex gap-2">
              <Button
                theme="light"
                size="SM"
                text={previewVisible ? 'Hide preview' : 'Show preview'}
                onClick={() => setPreviewVisible((v) => !v)}
              />
              <Button theme="light" size="SM" text="Back" onClick={() => navigate('/composers')} />
              <Button
                theme="light"
                size="SM"
                text="Export TOML"
                LeadingIcon={DocumentArrowDownIcon}
                onClick={() => setExportOpen(true)}
              />
              <Button
                theme="light"
                size="SM"
                text="Import TOML"
                LeadingIcon={DocumentArrowUpIcon}
                onClick={() => setImportOpen(true)}
              />
              <Button
                theme="light"
                size="SM"
                text="Restart"
                LeadingIcon={ArrowPathIcon}
                disabled={!isRestartable(data.status)}
                onClick={handleRestart}
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

          <ComposerLivePreview
            composerId={data.composer_id}
            visible={previewVisible}
            onToggle={() => setPreviewVisible(false)}
          />
          <ComposerCanvasEditor composer={data} />
          <ComposerInputsPanel composer={data} />
          <ComposerLayoutPanel composer={data} />
          <ComposerConsumersPanel composer={data} />
          <EntityLogsPanel
            filter={{ key: 'composer_id', id: data.composer_id }}
            description={`Live logs for composer ${data.composer_id}.`}
          />
        </div>

        <ComposerExportDialog
          composerId={data.composer_id}
          open={exportOpen}
          onClose={() => setExportOpen(false)}
        />

        <ComposerImportDialog
          open={importOpen}
          title={`Import TOML into "${data.composer_id}"`}
          description="Paste or load a composer TOML document. Its settings overwrite this composer; the id in the document is ignored."
          submitText="Overwrite"
          onClose={() => setImportOpen(false)}
          onImport={handleImport}
        />

        <ComposerDeleteDialog
          composerId={data.composer_id}
          streams={streamRefs}
          open={deleteOpen}
          onClose={() => setDeleteOpen(false)}
          onDeleted={() => navigate('/composers')}
        />
      </DashboardLayout.MainContent>
    </DashboardLayout>
  );
}
