import { useNavigate } from 'react-router-dom';
import { Badge } from '../Badge';
import { DataTable, type DataTableColumn } from '../primitives/DataTable';
import { EmptyState } from '../primitives/EmptyState';
import { StatusPill } from '../primitives/StatusPill';
import { poolStateToPill } from '../../lib/pool-status';
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
      id: 'status',
      header: 'Status',
      cell: (row) => <StatusPill status={poolStateToPill(row.status)} />,
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
