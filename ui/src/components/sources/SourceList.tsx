import { useNavigate } from 'react-router-dom';
import { Badge } from '../Badge';
import { DataTable, type DataTableColumn } from '../primitives/DataTable';
import { EmptyState } from '../primitives/EmptyState';
import { StatusPill } from '../primitives/StatusPill';
import { poolStateToPill, livenessToPill } from '../../lib/pool-status';
import type { SourceEntry } from '../../hooks/useSourceStore';
import { formatUptime } from '../../lib/formatUptime';

interface SourceListProps {
  sources: SourceEntry[];
}

export function SourceList({ sources }: Readonly<SourceListProps>) {
  const navigate = useNavigate();

  const columns: DataTableColumn<SourceEntry>[] = [
    {
      id: 'source_id',
      header: 'Source',
      cell: (row) => (
        <div className="flex flex-col">
          <span className="font-medium text-fg">{row.id}</span>
        </div>
      ),
      sortValue: (row) => row.id,
    },
    {
      id: 'device',
      header: 'Device',
      cell: (row) =>
        row.test_mode ? (
          <Badge tone="info" size="xs">test pattern</Badge>
        ) : (
          <span className="text-fg-muted text-xs font-mono">{row.device ?? '—'}</span>
        ),
      sortValue: (row) => (row.test_mode ? 'zzz-test-pattern' : row.device ?? ''),
    },
    {
      id: 'format',
      header: 'Format',
      cell: (row) => {
        const name = row.latest_status?.format?.fourcc ?? row.format?.format_name;
        return <span className="text-fg-muted text-xs font-mono uppercase">{name ?? '—'}</span>;
      },
      sortValue: (row) => row.latest_status?.format?.fourcc ?? row.format?.format_name ?? '',
    },
    {
      id: 'resolution',
      header: 'Res',
      cell: (row) => {
        const w = row.latest_status?.format?.w ?? row.format?.width;
        const h = row.latest_status?.format?.h ?? row.format?.height;
        return (
          <span className="text-fg-muted text-xs font-mono">
            {w && h ? `${w}×${h}` : '—'}
          </span>
        );
      },
      sortValue: (row) => row.latest_status?.format?.w ?? row.format?.width ?? 0,
    },
    {
      id: 'fps',
      header: 'FPS',
      cell: (row) => {
        const fps = row.effective_fps ?? row.latest_status?.format?.fps ?? row.format?.fps;
        return <span className="text-fg-muted text-xs font-mono">{fps ? fps : '—'}</span>;
      },
      sortValue: (row) => row.effective_fps ?? row.latest_status?.format?.fps ?? row.format?.fps ?? 0,
      align: 'right',
    },
    {
      id: 'status',
      header: 'Status',
      cell: (row) => <StatusPill status={poolStateToPill(row.status)} />,
      sortValue: (row) => row.status ?? '',
    },
    {
      id: 'liveness',
      header: 'Liveness',
      cell: (row) => {
        const pill = livenessToPill(row.liveness);
        return <StatusPill status={pill.status} label={pill.label} />;
      },
      sortValue: (row) => row.liveness ?? '',
    },
    {
      id: 'uptime',
      header: 'Uptime',
      cell: (row) => (
        <span className="text-fg-muted text-xs">{formatUptime(row.started_at_us) ?? '—'}</span>
      ),
      sortValue: (row) => row.started_at_us ?? 0,
    },
  ];

  return (
    <DataTable
      columns={columns}
      rows={sources}
      rowKey={(row) => row.id}
      onRowClick={(row) => navigate(`/sources/${row.id}`)}
      initialSort={{ columnId: 'source_id', direction: 'asc' }}
      emptyState={
        <EmptyState
          title="No sources yet"
          description="Sources appear here when devices are configured. Create a source from a connected V4L2 device, or enable a test-pattern source for dev/CI rigs."
        />
      }
    />
  );
}
