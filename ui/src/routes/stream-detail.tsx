import { useCallback, useEffect, useRef } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import toast from 'react-hot-toast';
import { ArrowPathIcon, PencilSquareIcon, TrashIcon } from '@heroicons/react/24/outline';
import { useAuthStore } from '../hooks/useAuthStore';
import { useStreamStore } from '../hooks/useStreamStore';
import { useSSEManager, type StreamLifecycleEvent } from '../hooks/useSSEManager';
import { DashboardLayout } from '../components/DashboardLayout';
import { InfoBar } from '../components/InfoBar';
import { Button } from '../components/Button';
import { EntityDetailLayout } from '../components/primitives/EntityDetailLayout';
import { StreamOverviewPanel } from '../components/streams/StreamOverviewPanel';
import { StreamMetricsPanel } from '../components/streams/StreamMetricsPanel';
import { StreamConsumersPanel } from '../components/streams/StreamConsumersPanel';
import { StreamEncoderPanel } from '../components/streams/StreamEncoderPanel';
import { StreamPublishPanel } from '../components/streams/StreamPublishPanel';
import { api } from '../lib/api';
import type { components } from '../lib/api.generated';

type StreamMetricsEvent = components['schemas']['StreamMetricsEvent'];

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
      } else if (event.type === 'canvas-restarted') {
        if (event.canvas) addStream(event.canvas);
        bumpStreamRefreshKey(event.canvas_id);
      }
    },
    [addStream, removeStream, bumpStreamRefreshKey, streamId, navigate],
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
              {stream.codec ? `${stream.codec.toLowerCase()} ` : ''}
              {stream.bitrate ?? ''}
            </span>
          }
          actions={
            <>
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
          <StreamOverviewPanel streamId={streamId} />
          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            <StreamMetricsPanel streamId={streamId} />
            <StreamEncoderPanel streamId={streamId} />
          </div>
          <StreamConsumersPanel streamId={streamId} />
          <StreamPublishPanel streamId={streamId} />
        </EntityDetailLayout>
      </DashboardLayout.MainContent>
    </DashboardLayout>
  );
}
