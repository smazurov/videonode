import { useCallback, useEffect, useRef, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import toast from 'react-hot-toast';
import {
  ArrowPathIcon,
  CheckIcon,
  ChevronDownIcon,
  EyeIcon,
  EyeSlashIcon,
  PencilSquareIcon,
  SwatchIcon,
  TrashIcon,
  VideoCameraIcon,
} from '@heroicons/react/24/outline';
import { Menu, MenuButton, MenuItems } from '@headlessui/react';
import { MenuRow } from '../components/menu/MenuRow';
import { useAuthStore } from '../hooks/useAuthStore';
import { useStreamStore } from '../hooks/useStreamStore';
import { useSSEManager } from '../hooks/useSSEManager';
import { DashboardLayout } from '../components/DashboardLayout';
import { InfoBar } from '../components/InfoBar';
import { Button } from '../components/Button';
import { EntityDetailLayout } from '../components/primitives/EntityDetailLayout';
import {
  useStreamPreviewMode,
  type StreamPreviewMode,
} from '../components/streams/useStreamPreviewMode';
import { StreamStatusPanel } from '../components/streams/StreamStatusPanel';
import { StreamOutputPanel } from '../components/streams/StreamOutputPanel';
import { StreamViewersPanel } from '../components/streams/StreamViewersPanel';
import { StreamPreviewPanel } from '../components/streams/StreamPreviewPanel';
import { EntityLogsPanel } from '../components/logs/EntityLogsPanel';
import { MaskExportDialog } from '../components/composers/MaskExportDialog';
import { useUpstreamComposerMask } from '../hooks/useComposerMask';

import { api } from '../lib/api';
import { isRestartable } from '../lib/pool-status';
import { latestHealth } from '../lib/stream-health';

const MODE_OPTIONS: ReadonlyArray<{
  value: StreamPreviewMode;
  label: string;
  Icon: typeof EyeIcon;
}> = [
  { value: 'large', label: 'Large', Icon: EyeIcon },
  { value: 'small', label: 'Small', Icon: VideoCameraIcon },
  { value: 'off', label: 'Off', Icon: EyeSlashIcon },
];

function PreviewModeToggle({
  mode,
  onChange,
}: {
  readonly mode: StreamPreviewMode;
  readonly onChange: (next: StreamPreviewMode) => void;
}) {
  const current = (MODE_OPTIONS.find((o) => o.value === mode) ?? MODE_OPTIONS[0])!;
  return (
    <Menu as="div" className="relative inline-block">
      <MenuButton
        as={Button}
        theme="light"
        size="SM"
        aria-label="Preview size"
        LeadingIcon={current.Icon}
        TrailingIcon={ChevronDownIcon}
        text={`Preview: ${current.label}`}
      />
      <MenuItems
        anchor="bottom end"
        className="z-50 mt-1 min-w-[160px] rounded border border-border bg-surface-raised py-1 shadow-lg focus:outline-none"
      >
        {MODE_OPTIONS.map((opt) => (
          <MenuRow
            key={opt.value}
            icon={opt.Icon}
            label={opt.label}
            onClick={() => onChange(opt.value)}
            trailing={opt.value === mode ? <CheckIcon className="h-4 w-4 text-fg" /> : null}
          />
        ))}
      </MenuItems>
    </Menu>
  );
}

// Status and Output side by side (like a source's Status and Format). The
// small preview takes the left column from lg; stacked, Status comes first so
// health is never pushed below the video. The large preview spans the top.
function StreamOverview({
  streamId,
  previewMode,
  canPreview,
  refreshKey,
  onHidePreview,
}: {
  readonly streamId: string;
  readonly previewMode: StreamPreviewMode;
  readonly canPreview: boolean;
  readonly refreshKey: number;
  readonly onHidePreview: () => void;
}) {
  const preview = (large: boolean) => (
    <StreamPreviewPanel
      streamId={streamId}
      canPreview={canPreview}
      refreshKey={refreshKey}
      onHide={onHidePreview}
      showStats={large}
    />
  );
  if (previewMode === 'small') {
    return (
      <div className="grid grid-cols-1 items-start gap-4 lg:grid-cols-2">
        <div className="lg:col-start-2 lg:row-start-1"><StreamStatusPanel streamId={streamId} /></div>
        <div className="lg:col-start-1 lg:row-span-2 lg:row-start-1">{preview(false)}</div>
        <div className="lg:col-start-2 lg:row-start-2"><StreamOutputPanel streamId={streamId} /></div>
      </div>
    );
  }
  return (
    <>
      {previewMode === 'large' && preview(true)}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <StreamStatusPanel streamId={streamId} />
        <StreamOutputPanel streamId={streamId} />
      </div>
    </>
  );
}

export default function StreamDetail() {
  const navigate = useNavigate();
  const { streamId } = useParams<{ streamId: string }>();
  const logout = useAuthStore((s) => s.logout);

  const stream = useStreamStore((state) => (streamId ? state.streamsById[streamId] : undefined));
  const metrics = useStreamStore((state) => (streamId ? state.metricsById[streamId] : undefined));
  const lastUpdated = useStreamStore((state) => state.lastUpdated);
  const fetchStreams = useStreamStore((state) => state.fetchStreams);
  const deleteStreamAction = useStreamStore((state) => state.deleteStream);
  const bumpStreamRefreshKey = useStreamStore((state) => state.bumpStreamRefreshKey);
  const refreshKey = useStreamStore(
    (state) => (streamId ? state.streamRefreshKeys[streamId] ?? 0 : 0),
  );
  const pipelineEnabled = useStreamStore((state) => state.pipelineEnabled);

  const { mode: previewMode, setMode: setPreviewMode } = useStreamPreviewMode(streamId ?? '');
  const mask = useUpstreamComposerMask(streamId ?? '');
  const [maskOpen, setMaskOpen] = useState(false);

  useEffect(() => {
    if (lastUpdated === null) {
      fetchStreams();
    }
  }, [lastUpdated, fetchStreams]);

  useEffect(() => {
    const missing = !streamId || (lastUpdated !== null && !stream);
    if (missing) navigate('/streams');
  }, [streamId, stream, lastUpdated, navigate]);

  const prevConnectionStatusRef = useRef<'online' | 'offline' | 'reconnecting'>('online');
  const handleConnectionStatus = useCallback(
    (status: 'online' | 'offline' | 'reconnecting') => {
      if (status === 'online' && prevConnectionStatusRef.current !== 'online') {
        fetchStreams();
      }
      prevConnectionStatusRef.current = status;
    },
    [fetchStreams],
  );

  useSSEManager({
    onConnectionStatusChange: handleConnectionStatus,
  });

  const handleRestart = useCallback(async () => {
    if (!streamId) return;
    try {
      const { error } = await api.POST('/api/processes/{id}/restart', {
        params: { path: { id: `encoder:${streamId}` } },
      });
      if (error) throw new Error(error.detail ?? 'Failed to restart stream');
      bumpStreamRefreshKey(streamId);
      toast.success(`Restart requested for '${streamId}'`);
    } catch (error) {
      console.error('Failed to restart stream:', error);
      toast.error('Failed to restart stream');
    }
  }, [streamId, bumpStreamRefreshKey]);

  const handleDelete = useCallback(async () => {
    if (!streamId) return;
    if (!window.confirm(`Delete stream '${streamId}'? This cannot be undone.`)) return;
    try {
      await deleteStreamAction(streamId);
      navigate('/streams');
    } catch (error) {
      console.error('Failed to delete stream:', error);
      toast.error('Failed to delete stream');
    }
  }, [streamId, deleteStreamAction, navigate]);

  if (!streamId || lastUpdated === null || !stream) {
    return (
      <DashboardLayout onLogout={logout} bottomBar={<InfoBar />}>
        <DashboardLayout.MainContent>
          <div className="flex h-64 items-center justify-center">
            <div className="h-8 w-8 animate-spin rounded-full border-b-2 border-fg" />
          </div>
        </DashboardLayout.MainContent>
      </DashboardLayout>
    );
  }

  // Mount the player whenever the pipeline is on, even if the stream is idle:
  // the offer it sends is what lazily wakes the encoder (lazy-encoder-on-reader).
  const canPreview = pipelineEnabled !== false;
  // stream.status in the store only refreshes on refetch, so it stays "idle"
  // after a viewer lazily starts the encoder; the health push carries the
  // live pool state every second.
  const encoderState = latestHealth(stream.health, metrics?.health)?.encoder.state ?? stream.status;

  const actions = (
    <>
      <PreviewModeToggle mode={previewMode} onChange={setPreviewMode} />
      <Button
        theme="light"
        size="SM"
        onClick={handleRestart}
        disabled={!isRestartable(encoderState)}
        LeadingIcon={ArrowPathIcon}
        text="Restart"
      />
      <Button
        theme="light"
        size="SM"
        onClick={() => navigate(`/streams/${encodeURIComponent(streamId)}/edit`)}
        LeadingIcon={PencilSquareIcon}
        text="Edit"
      />
      {mask && (
        <Button
          theme="light"
          size="SM"
          onClick={() => setMaskOpen(true)}
          LeadingIcon={SwatchIcon}
          text="OBS mask"
        />
      )}
      <Button
        theme="danger"
        size="SM"
        onClick={handleDelete}
        LeadingIcon={TrashIcon}
        text="Delete"
      />
    </>
  );

  return (
    <DashboardLayout onLogout={logout} bottomBar={<InfoBar />}>
      <DashboardLayout.MainContent>
        <EntityDetailLayout
          breadcrumbs={[
            { label: 'Streams', to: '/streams' },
            { label: streamId },
          ]}
          title={streamId}
          actions={actions}
        >
          <StreamOverview
            streamId={streamId}
            previewMode={previewMode}
            canPreview={canPreview}
            refreshKey={refreshKey}
            onHidePreview={() => setPreviewMode('off')}
          />
          <StreamViewersPanel streamId={streamId} />
          <EntityLogsPanel filter={{ key: 'stream_id', id: streamId }} />
        </EntityDetailLayout>

        {mask && (
          <MaskExportDialog
            mask={mask}
            streamId={streamId}
            open={maskOpen}
            onClose={() => setMaskOpen(false)}
          />
        )}
      </DashboardLayout.MainContent>
    </DashboardLayout>
  );
}
