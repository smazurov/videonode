import { useCallback, useEffect, useRef } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import toast from 'react-hot-toast';
import {
  ArrowPathIcon,
  CheckIcon,
  ChevronDownIcon,
  EyeIcon,
  EyeSlashIcon,
  PencilSquareIcon,
  TrashIcon,
  VideoCameraIcon,
} from '@heroicons/react/24/outline';
import { Menu, MenuButton, MenuItems } from '@headlessui/react';
import { MenuRow } from '../components/menu/MenuRow';
import { useAuthStore } from '../hooks/useAuthStore';
import { useStreamStore } from '../hooks/useStreamStore';
import { useSSEManager, type StreamLifecycleEvent } from '../hooks/useSSEManager';
import { DashboardLayout } from '../components/DashboardLayout';
import { InfoBar } from '../components/InfoBar';
import { Button } from '../components/Button';
import { EntityDetailLayout } from '../components/primitives/EntityDetailLayout';
import { LivePreviewFrame } from '../components/primitives/LivePreviewFrame';
import { StreamOverviewPanel } from '../components/streams/StreamOverviewPanel';
import {
  useStreamPreviewMode,
  type StreamPreviewMode,
} from '../components/streams/useStreamPreviewMode';
import { StreamMetricsPanel } from '../components/streams/StreamMetricsPanel';
import { StreamConsumersPanel } from '../components/streams/StreamConsumersPanel';
import { StreamEncoderPanel } from '../components/streams/StreamEncoderPanel';
import { StreamPublishPanel } from '../components/streams/StreamPublishPanel';
import { WebRTCPlayer } from '../components/webrtc';
import { api } from '../lib/api';
import type { components } from '../lib/api.generated';

type StreamMetricsEvent = components['schemas']['StreamMetricsEvent'];

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

export default function StreamDetail() {
  const navigate = useNavigate();
  const { streamId } = useParams<{ streamId: string }>();
  const { logout } = useAuthStore();

  const stream = useStreamStore((state) => (streamId ? state.streamsById[streamId] : undefined));
  const lastUpdated = useStreamStore((state) => state.lastUpdated);
  const fetchStreams = useStreamStore((state) => state.fetchStreams);
  const addStream = useStreamStore((state) => state.addStream);
  const removeStream = useStreamStore((state) => state.removeStream);
  const deleteStreamAction = useStreamStore((state) => state.deleteStream);
  const updateStreamMetrics = useStreamStore((state) => state.updateStreamMetrics);
  const bumpStreamRefreshKey = useStreamStore((state) => state.bumpStreamRefreshKey);
  const refreshKey = useStreamStore(
    (state) => (streamId ? state.streamRefreshKeys[streamId] ?? 0 : 0),
  );
  const pipelineEnabled = useStreamStore((state) => state.pipelineEnabled);

  const { mode: previewMode, setMode: setPreviewMode } = useStreamPreviewMode(streamId ?? '');

  useEffect(() => {
    if (lastUpdated === null) {
      fetchStreams();
    }
  }, [lastUpdated, fetchStreams]);

  useEffect(() => {
    const missing = !streamId || (lastUpdated !== null && !stream);
    if (missing) navigate('/streams');
  }, [streamId, stream, lastUpdated, navigate]);

  const handleStreamLifecycle = useCallback(
    (event: StreamLifecycleEvent) => {
      if (event.type === 'stream-created' || event.type === 'stream-updated') {
        addStream(event.stream);
      } else if (event.type === 'stream-deleted' && event.stream_id === streamId) {
        removeStream(event.stream_id);
        navigate('/streams');
      }
    },
    [addStream, removeStream, streamId, navigate],
  );

  const handleStreamMetrics = useCallback(
    (event: StreamMetricsEvent) => {
      updateStreamMetrics(event);
    },
    [updateStreamMetrics],
  );

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
    onStreamLifecycleEvent: handleStreamLifecycle,
    onStreamMetricsEvent: handleStreamMetrics,
    onConnectionStatusChange: handleConnectionStatus,
  });

  const handleRestart = useCallback(async () => {
    if (!streamId) return;
    try {
      const { error } = await api.POST('/api/streams/{stream_id}/restart', {
        params: { path: { stream_id: streamId } },
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

  const isRunning = stream.status === 'running';
  const showSmallPreview = previewMode === 'small';
  const previewIdleMessage = pipelineEnabled === false ? 'Pipeline stopped' : 'Stream disabled';

  return (
    <DashboardLayout onLogout={logout} bottomBar={<InfoBar />}>
      <DashboardLayout.MainContent>
        <EntityDetailLayout
          breadcrumbs={[
            { label: 'Streams', to: '/streams' },
            { label: streamId },
          ]}
          title={streamId}
          subtitle={
            <span className="font-mono text-xs">
              {stream.encoder?.codec ? `${stream.encoder.codec.toLowerCase()} ` : ''}
              {stream.encoder?.bitrate ?? ''}
            </span>
          }
          actions={
            <>
              <PreviewModeToggle mode={previewMode} onChange={setPreviewMode} />
              <Button
                theme="light"
                size="SM"
                onClick={handleRestart}
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
              <Button
                theme="danger"
                size="SM"
                onClick={handleDelete}
                LeadingIcon={TrashIcon}
                text="Delete"
              />
            </>
          }
        >
          <StreamOverviewPanel streamId={streamId} videoHidden={previewMode !== 'large'} />
          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            <StreamMetricsPanel streamId={streamId} />
            <StreamEncoderPanel streamId={streamId} />
          </div>
          {showSmallPreview ? (
            <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
              <div className="lg:col-span-1">
                <section className="space-y-2 rounded-lg border border-border bg-surface-raised p-3">
                  <div className="flex items-center justify-between">
                    <span className="text-xs uppercase tracking-wide text-fg-muted">
                      Preview
                    </span>
                    <span className="text-[10px] uppercase tracking-wide text-fg-subtle">
                      WebRTC
                    </span>
                  </div>
                  <LivePreviewFrame
                    state={isRunning ? 'ready' : 'idle'}
                    idleMessage={previewIdleMessage}
                    mediaClassName="bg-black"
                  >
                    {isRunning && (
                      <WebRTCPlayer
                        key={`small:${streamId}:${refreshKey}`}
                        streamId={streamId}
                        className="w-full h-full"
                        muted
                      />
                    )}
                  </LivePreviewFrame>
                </section>
              </div>
              <div className="lg:col-span-2">
                <StreamConsumersPanel streamId={streamId} />
              </div>
            </div>
          ) : (
            <StreamConsumersPanel streamId={streamId} />
          )}
          <StreamPublishPanel streamId={streamId} />
        </EntityDetailLayout>
      </DashboardLayout.MainContent>
    </DashboardLayout>
  );
}
