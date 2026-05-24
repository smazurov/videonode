import { useNavigate } from 'react-router-dom';
import { Badge } from '../Badge';
import { DataTable, type DataTableColumn } from '../primitives/DataTable';
import { EmptyState } from '../primitives/EmptyState';
import { StatusPill } from '../primitives/StatusPill';
import type { SourceEntry } from '../../hooks/useSourceStore';

interface SourceListProps {
  sources: SourceEntry[];
}

function formatRunningSince(iso?: string): string {
  if (!iso) return '—';
  const ts = new Date(iso);
  if (Number.isNaN(ts.getTime())) return '—';
  const diffMs = Date.now() - ts.getTime();
  if (diffMs < 0) return ts.toLocaleString();
  const sec = Math.floor(diffMs / 1000);
  if (sec < 60) return `${sec}s ago`;
  const min = Math.floor(sec / 60);
  if (min < 60) return `${min}m ago`;
  const hr = Math.floor(min / 60);
  if (hr < 24) return `${hr}h ago`;
  const day = Math.floor(hr / 24);
  return `${day}d ago`;
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
      id: 'status',
      header: 'Status',
      cell: (row) => <StatusPill status={row.status ?? 'idle'} />,
      sortValue: (row) => row.status ?? '',
    },
    {
      id: 'consumers',
      header: 'Consumers',
      cell: (row) => <span className="text-fg">{row.consumer_count ?? row.consumers?.length ?? 0}</span>,
      sortValue: (row) => row.consumer_count ?? row.consumers?.length ?? 0,
      className: 'text-right',
    },
    {
      id: 'running_since',
      header: 'Running since',
      cell: (row) => (
        <span className="text-fg-muted text-xs">{formatRunningSince(row.running_since)}</span>
      ),
      sortValue: (row) => row.running_since ?? '',
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
