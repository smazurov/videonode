import { useMemo } from 'react';
import { useNavigate, Link } from 'react-router-dom';
import {
  ExclamationTriangleIcon,
  PlusIcon,
  VideoCameraIcon,
} from '@heroicons/react/24/outline';
import { Button } from '../Button';
import { Card } from '../Card';
import { Badge } from '../Badge';
import { DataTable, type DataTableColumn } from '../primitives/DataTable';
import { StatusPill, type StatusPillStatus } from '../primitives/StatusPill';
import { poolStateToPill } from '../../lib/pool-status';
import { useStreamStore } from '../../hooks/useStreamStore';
import { buildStreamURL } from '../../lib/api';
import type { components } from '../../lib/api.generated';
import { resolveUpstream, type UpstreamRef } from './upstream';

type StreamData = components['schemas']['StreamData'];

interface StreamRow {
  readonly streamId: string;
  readonly stream: StreamData;
  readonly upstream: UpstreamRef;
}

interface StreamListProps {
  readonly streamIds: ReadonlyArray<string>;
  readonly loading?: boolean;
  readonly error?: string | null;
  readonly onRefresh?: () => void;
  readonly onCreateStream?: () => void;
}

function encoderStatusTone(stream: StreamData): { status: StatusPillStatus; label: string } {
  const s = poolStateToPill(stream.status);
  return { status: s, label: s };
}

function codecBitrate(stream: StreamData): string {
  const codec = stream.encoder?.codec || '—';
  const bitrate = stream.encoder?.bitrate;
  if (!bitrate) return codec.toLowerCase();
  return `${codec.toLowerCase()} ${bitrate}`;
}

function UpstreamBadge({ upstream }: { readonly upstream: UpstreamRef }) {
  const tone = upstream.kind === 'composer' ? 'canvas' : 'info';
  const content = (
    <Badge tone={tone} size="sm">
      {upstream.raw}
    </Badge>
  );
  if (!upstream.href) return content;
  return (
    <Link
      to={upstream.href}
      onClick={(e) => e.stopPropagation()}
      className="inline-flex hover:opacity-80"
    >
      {content}
    </Link>
  );
}

function URLCell({ url, protocol, tone }: { readonly url: string | undefined; readonly protocol: 'rtsp' | 'srt'; readonly tone: 'rtsp' | 'srt' }) {
  if (!url) return <span className="text-xs text-fg-subtle">—</span>;
  const full = buildStreamURL(url, protocol) ?? url;
  return (
    <span className="inline-flex items-center gap-2">
      <Badge tone={tone} size="xs">{protocol.toUpperCase()}</Badge>
      <code className="max-w-[200px] truncate font-mono text-xs text-fg-muted" title={full}>
        {full}
      </code>
    </span>
  );
}

export function StreamList({
  streamIds,
  loading = false,
  error = null,
  onRefresh,
  onCreateStream,
}: StreamListProps) {
  const navigate = useNavigate();
  const streamsById = useStreamStore((state) => state.streamsById);
  const consumersById = useStreamStore((state) => state.consumersById);

  const rows = useMemo<StreamRow[]>(() => {
    return streamIds
      .map((id) => {
        const stream = streamsById[id];
        if (!stream) return null;
        return { streamId: id, stream, upstream: resolveUpstream(stream) };
      })
      .filter((r): r is StreamRow => r !== null);
  }, [streamIds, streamsById]);

  const columns: DataTableColumn<StreamRow>[] = useMemo(
    () => [
      {
        id: 'stream_id',
        header: 'Stream',
        cell: (row) => <span className="font-mono text-sm font-medium text-fg">{row.streamId}</span>,
        sortValue: (row) => row.streamId,
      },
      {
        id: 'upstream',
        header: 'Upstream',
        cell: (row) => <UpstreamBadge upstream={row.upstream} />,
        sortValue: (row) => row.upstream.raw,
      },
      {
        id: 'codec',
        header: 'Codec / Bitrate',
        cell: (row) => (
          <span className="font-mono text-xs uppercase text-fg-muted">{codecBitrate(row.stream)}</span>
        ),
        sortValue: (row) => codecBitrate(row.stream),
      },
      {
        id: 'status',
        header: 'Encoder',
        cell: (row) => {
          const { status, label } = encoderStatusTone(row.stream);
          return <StatusPill status={status}>{label}</StatusPill>;
        },
        sortValue: (row) => (row.stream.enabled ? 0 : 1),
      },
      {
        id: 'readers',
        header: 'Readers',
        cell: (row) => {
          const c = consumersById[row.streamId];
          const total = c?.total ?? 0;
          const breakdown = c
            ? `RTSP ${c.rtsp ?? 0} · WebRTC ${c.webrtc ?? 0} · SRT ${c.srt ?? 0}`
            : 'no readers yet';
          return (
            <span className="tabular-nums text-sm" title={breakdown}>
              {total}
            </span>
          );
        },
        sortValue: (row) => consumersById[row.streamId]?.total ?? 0,
        className: 'text-right',
      },
      {
        id: 'rtsp',
        header: 'RTSP',
        cell: (row) => <URLCell url={row.stream.rtsp_url} protocol="rtsp" tone="rtsp" />,
      },
      {
        id: 'srt',
        header: 'SRT',
        cell: (row) => <URLCell url={row.stream.srt_url} protocol="srt" tone="srt" />,
      },
      {
        id: 'webrtc',
        header: 'WebRTC',
        cell: (row) => (
          <Link
            to={`/video?stream=${encodeURIComponent(row.streamId)}`}
            onClick={(e) => e.stopPropagation()}
            className="inline-flex items-center gap-2"
          >
            <Badge tone="webrtc" size="xs">WebRTC</Badge>
            <span className="text-xs text-fg-muted underline-offset-2 hover:underline">open</span>
          </Link>
        ),
      },
    ],
    [consumersById],
  );

  const handleRowClick = (row: StreamRow) => {
    navigate(`/streams/${encodeURIComponent(row.streamId)}`);
  };

  const header = (
    <div className="flex items-center justify-between">
      <div>
        <h2 className="text-2xl font-bold text-fg">Video Streams</h2>
        <p className="mt-1 text-fg-muted">
          {streamIds.length} active {streamIds.length === 1 ? 'stream' : 'streams'}
        </p>
      </div>
      <div className="flex items-center gap-2">
        {onRefresh && (
          <Button
            theme="light"
            size="MD"
            onClick={onRefresh}
            disabled={loading}
            text={loading ? 'Refreshing…' : 'Refresh'}
          />
        )}
        {onCreateStream && (
          <Button
            onClick={onCreateStream}
            theme="primary"
            size="MD"
            LeadingIcon={PlusIcon}
            text="Create Stream"
          />
        )}
      </div>
    </div>
  );

  if (loading && rows.length === 0) {
    return (
      <div className="space-y-6">
        {header}
        <Card className="py-12 text-center">
          <Card.Content>
            <div className="mx-auto h-8 w-8 animate-spin rounded-full border-b-2 border-fg" />
            <p className="mt-4 text-fg-muted">Loading streams…</p>
          </Card.Content>
        </Card>
      </div>
    );
  }

  if (error) {
    return (
      <div className="space-y-6">
        {header}
        <Card className="py-12 text-center">
          <Card.Content>
            <div className="mx-auto mb-4 flex h-16 w-16 items-center justify-center rounded-full bg-danger-soft">
              <ExclamationTriangleIcon className="h-8 w-8 text-danger-soft-fg" />
            </div>
            <h3 className="mb-2 text-lg font-medium text-fg">Failed to load streams</h3>
            <p className="mb-6 text-fg-muted">{error}</p>
            {onRefresh && <Button onClick={onRefresh} theme="light" size="MD" text="Try Again" />}
          </Card.Content>
        </Card>
      </div>
    );
  }

  if (rows.length === 0) {
    return (
      <div className="space-y-6">
        {header}
        <Card className="py-12 text-center">
          <Card.Content>
            <div className="mx-auto mb-4 flex h-16 w-16 items-center justify-center rounded-full bg-surface-muted">
              <VideoCameraIcon className="h-8 w-8 text-fg-subtle" />
            </div>
            <h3 className="mb-2 text-lg font-medium text-fg">No active streams</h3>
            <p className="mb-6 text-fg-muted">Create your first video stream to get started</p>
            {onCreateStream && (
              <Button
                onClick={onCreateStream}
                theme="primary"
                size="LG"
                LeadingIcon={PlusIcon}
                text="Create Stream"
              />
            )}
          </Card.Content>
        </Card>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {header}
      <DataTable<StreamRow>
        rows={rows}
        columns={columns}
        rowKey={(row) => row.streamId}
        onRowClick={handleRowClick}
        initialSort={{ columnId: 'stream_id', direction: 'asc' }}
      />
    </div>
  );
}
