import { useCallback, useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import toast from 'react-hot-toast';
import { StopIcon, TrashIcon } from '@heroicons/react/24/outline';
import { useAuthStore } from '../hooks/useAuthStore';
import { useStreamStore } from '../hooks/useStreamStore';
import { useRecordingStore } from '../hooks/useRecordingStore';
import { DashboardLayout } from '../components/DashboardLayout';
import { InfoBar } from '../components/InfoBar';
import { EntityDetailLayout } from '../components/primitives/EntityDetailLayout';
import { Button } from '../components/Button';
import { RecordingPlayer } from '../components/streams/RecordingPlayer';
import { api } from '../lib/api';
import { formatStarted } from '../components/streams/format';
import type { components } from '../lib/api.generated';

type Recording = components['schemas']['RecordingStatusData'];

// RecordingDetail plays back a single recording session; an active session
// can be stopped here, a finished one deleted. Recordings are created on a
// stream's detail page.
export default function RecordingDetail() {
  const navigate = useNavigate();
  const logout = useAuthStore((s) => s.logout);
  const { streamId, session } = useParams<{ streamId: string; session: string }>();

  const status: Recording | null = useRecordingStore(
    (s) => s.recordingsById[`${streamId}/${session}`] ?? null,
  );
  const loaded = useRecordingStore((s) => s.loaded);
  const fetchRecordings = useRecordingStore((s) => s.fetchRecordings);
  const removeRecording = useRecordingStore((s) => s.removeRecording);
  const loading = !loaded;
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!loaded) void fetchRecordings();
  }, [loaded, fetchRecordings]);

  const stopRecording = useStreamStore((s) => s.stopRecording);
  const upsertRecording = useRecordingStore((s) => s.upsertRecording);

  const handleStop = useCallback(async () => {
    if (!streamId) return;
    setBusy(true);
    try {
      upsertRecording(await stopRecording(streamId));
      toast.success('Recording stopped');
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Failed to stop recording');
    } finally {
      setBusy(false);
    }
  }, [streamId, stopRecording, upsertRecording]);

  const handleDelete = useCallback(async () => {
    if (!streamId || !session) return;
    if (!window.confirm('Delete this recording? This permanently removes its files.')) return;
    setBusy(true);
    const { error } = await api.DELETE('/api/streams/{stream_id}/recordings/{session}', {
      params: { path: { stream_id: streamId, session } },
    });
    setBusy(false);
    if (error) {
      toast.error(error.detail ?? 'Failed to delete recording');
      return;
    }
    toast.success('Recording deleted');
    removeRecording(`${streamId}/${session}`);
    navigate('/recordings');
  }, [streamId, session, navigate, removeRecording]);

  return (
    <DashboardLayout onLogout={logout} bottomBar={<InfoBar />}>
      <DashboardLayout.MainContent>
        <EntityDetailLayout
          breadcrumbs={[{ label: 'Recordings', to: '/recordings' }, { label: streamId ?? '' }]}
          title={session ?? ''}
          subtitle={
            <span className="font-mono text-xs">
              {streamId}
              {status?.started_at ? ` · ${formatStarted(status.started_at, '')}` : ''}
            </span>
          }
          actions={
            <div className="flex items-center gap-2">
              {status?.active && (
                <Button
                  theme="danger"
                  size="SM"
                  onClick={handleStop}
                  disabled={busy}
                  LeadingIcon={StopIcon}
                  text="Stop"
                />
              )}
              <Button
                theme={status?.active ? 'light' : 'danger'}
                size="SM"
                onClick={handleDelete}
                disabled={busy || status?.active === true}
                title={status?.active ? 'Stop the recording before deleting' : undefined}
                LeadingIcon={TrashIcon}
                text="Delete"
              />
            </div>
          }
        >
          {status ? (
            <section className="rounded-lg border border-border bg-surface-raised p-4">
              <RecordingPlayer status={status} />
            </section>
          ) : (
            <section className="rounded-lg border border-border bg-surface-raised p-6">
              <p className="text-sm text-fg-muted">
                {loading ? 'Loading…' : 'Recording not found. It may have been deleted.'}
              </p>
            </section>
          )}
        </EntityDetailLayout>
      </DashboardLayout.MainContent>
    </DashboardLayout>
  );
}
