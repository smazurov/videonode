import { useCallback, useEffect, useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import toast from 'react-hot-toast';
import { ArrowPathIcon, TrashIcon } from '@heroicons/react/24/outline';
import { useAuthStore } from '../hooks/useAuthStore';
import { useRecordingStore } from '../hooks/useRecordingStore';
import { DashboardLayout } from '../components/DashboardLayout';
import { InfoBar } from '../components/InfoBar';
import { DataTable, type DataTableColumn } from '../components/primitives/DataTable';
import { Badge } from '../components/Badge';
import { Button } from '../components/Button';
import { api, API_BASE_URL } from '../lib/api';
import type { components } from '../lib/api.generated';

type Recording = components['schemas']['RecordingStatusData'];

function formatStarted(iso: string | undefined): string {
  if (!iso) return '—';
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? '—' : d.toLocaleString();
}

function formatBytes(bytes: number | undefined): string {
  if (!bytes || bytes <= 0) return '—';
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`;
}

function formatLength(sec: number | undefined): string {
  if (!sec || sec <= 0) return '—';
  const s = Math.round(sec);
  const m = Math.floor(s / 60);
  return `${String(m).padStart(2, '0')}:${String(s % 60).padStart(2, '0')}`;
}

// Recordings lists every recording session (active in-memory + completed on
// disk), kept live by recording.* SSE events. Rows link to the detail route
// for playback/scrub and delete. Creating recordings happens on a stream's
// detail page, not here.
export default function Recordings() {
  const navigate = useNavigate();
  const logout = useAuthStore((s) => s.logout);

  const recordingsById = useRecordingStore((s) => s.recordingsById);
  const loaded = useRecordingStore((s) => s.loaded);
  const fetchRecordings = useRecordingStore((s) => s.fetchRecordings);
  const removeRecording = useRecordingStore((s) => s.removeRecording);

  const rows = useMemo(() => Object.values(recordingsById), [recordingsById]);
  const loading = !loaded;

  const refresh = useCallback(() => {
    void fetchRecordings();
  }, [fetchRecordings]);

  useEffect(() => {
    void fetchRecordings();
  }, [fetchRecordings]);

  const handleDelete = useCallback(
    async (r: Recording) => {
      if (!window.confirm(`Delete recording ${r.recording_id} of ${r.stream_id}?`)) return;
      const { error } = await api.DELETE('/api/streams/{stream_id}/recordings/{session}', {
        params: { path: { stream_id: r.stream_id, session: r.recording_id } },
      });
      if (error) {
        toast.error(error.detail ?? 'Failed to delete recording');
        return;
      }
      toast.success('Recording deleted');
      removeRecording(`${r.stream_id}/${r.recording_id}`);
    },
    [removeRecording],
  );

  const columns: DataTableColumn<Recording>[] = [
    {
      id: 'preview',
      header: '',
      cell: (r) => (
        <img
          src={`${API_BASE_URL}/api/streams/${encodeURIComponent(r.stream_id)}/recordings/${encodeURIComponent(r.recording_id)}/poster.jpg`}
          alt=""
          loading="lazy"
          className="h-10 w-auto rounded border border-border"
          onError={(e) => {
            e.currentTarget.style.visibility = 'hidden';
          }}
        />
      ),
      width: '88px',
    },
    {
      id: 'stream',
      header: 'Stream',
      cell: (r) => <span className="font-mono text-sm font-medium text-fg">{r.stream_id}</span>,
      sortValue: (r) => r.stream_id,
    },
    {
      id: 'started',
      header: 'Started',
      cell: (r) => <span className="text-sm text-fg-muted">{formatStarted(r.started_at)}</span>,
      sortValue: (r) => r.started_at ?? '',
      width: '220px',
    },
    {
      id: 'length',
      header: 'Length',
      cell: (r) => <span className="tabular-nums text-sm">{formatLength(r.duration_seconds)}</span>,
      sortValue: (r) => r.duration_seconds ?? 0,
      align: 'right',
      width: '100px',
    },
    {
      id: 'segments',
      header: 'Segments',
      cell: (r) => <span className="tabular-nums text-sm">{r.segments ?? 0}</span>,
      sortValue: (r) => r.segments ?? 0,
      align: 'right',
      width: '110px',
    },
    {
      id: 'size',
      header: 'Size',
      cell: (r) => <span className="tabular-nums text-sm">{formatBytes(r.size_bytes)}</span>,
      sortValue: (r) => r.size_bytes ?? 0,
      align: 'right',
      width: '110px',
    },
    {
      id: 'status',
      header: 'Status',
      cell: (r) =>
        r.active ? (
          <Badge tone="danger" size="sm">
            REC
          </Badge>
        ) : (
          <Badge tone="neutral" size="sm">
            done
          </Badge>
        ),
      sortValue: (r) => (r.active ? 0 : 1),
      width: '100px',
    },
    {
      id: 'actions',
      header: '',
      cell: (r) => (
        <Button
          theme="blank"
          size="SM"
          LeadingIcon={TrashIcon}
          disabled={r.active}
          title={r.active ? 'Stop the recording before deleting' : 'Delete recording'}
          onClick={(e) => {
            e.stopPropagation();
            void handleDelete(r);
          }}
        />
      ),
      align: 'right',
      width: '64px',
    },
  ];

  const handleRowClick = useCallback(
    (r: Recording) => {
      navigate(
        `/recordings/${encodeURIComponent(r.stream_id)}/${encodeURIComponent(r.recording_id)}`,
      );
    },
    [navigate],
  );

  return (
    <DashboardLayout onLogout={logout} bottomBar={<InfoBar />}>
      <DashboardLayout.MainContent>
        <div className="flex items-center justify-between">
          <h1 className="text-lg font-semibold text-fg">Recordings</h1>
          <Button
            theme="light"
            size="SM"
            LeadingIcon={ArrowPathIcon}
            text="Refresh"
            onClick={refresh}
          />
        </div>
        <DataTable<Recording>
          rows={rows}
          columns={columns}
          rowKey={(r) => `${r.stream_id}/${r.recording_id}`}
          onRowClick={handleRowClick}
          initialSort={{ columnId: 'started', direction: 'desc' }}
          emptyState={
            loading ? 'Loading…' : 'No recordings yet. Start one from a stream’s detail page.'
          }
        />
      </DashboardLayout.MainContent>
    </DashboardLayout>
  );
}
