import { useNavigate } from 'react-router-dom';

import { Badge } from '../Badge';
import { DataTable, type DataTableColumn } from '../primitives/DataTable';
import { EmptyState } from '../primitives/EmptyState';
import { StatusPill } from '../primitives/StatusPill';
import { poolStateToPill } from '../../lib/pool-status';
import type { Sensor } from '../../hooks/useSensorStore';

interface SensorListProps {
  sensors: Sensor[];
}

function findingSummary(sensor: Sensor): string {
  const f = sensor.latest_finding;
  if (!f) return '—';
  const conf = typeof f.confidence === 'number' ? ` ${(f.confidence * 100).toFixed(0)}%` : '';
  return `${f.decision ?? f.kind ?? 'finding'}${conf}`;
}

export function SensorList({ sensors }: Readonly<SensorListProps>) {
  const navigate = useNavigate();

  const columns: DataTableColumn<Sensor>[] = [
    {
      id: 'sensor_id',
      header: 'Sensor',
      cell: (row) => <span className="font-medium text-fg">{row.id}</span>,
      sortValue: (row) => row.id,
    },
    {
      id: 'source',
      header: 'Observes',
      cell: (row) => <span className="text-fg-muted text-xs font-mono">{row.source ?? '—'}</span>,
      sortValue: (row) => row.source ?? '',
    },
    {
      id: 'mode',
      header: 'Mode',
      cell: (row) => (
        <Badge tone={row.mode === 'propose' ? 'warning' : 'info'} size="xs">
          {row.mode ?? 'auto'}
        </Badge>
      ),
      sortValue: (row) => row.mode ?? 'auto',
    },
    {
      id: 'bindings',
      header: 'Bound to',
      cell: (row) => (
        <span className="text-fg-muted text-xs">
          {row.bindings?.length ? `${row.bindings.length} input(s)` : 'unattached'}
        </span>
      ),
      sortValue: (row) => row.bindings?.length ?? 0,
      align: 'right',
    },
    {
      id: 'finding',
      header: 'Last finding',
      cell: (row) => <span className="text-fg-muted text-xs font-mono">{findingSummary(row)}</span>,
      sortValue: (row) => row.latest_finding?.frame_idx ?? 0,
    },
    {
      id: 'status',
      header: 'Status',
      cell: (row) => <StatusPill status={poolStateToPill(row.status)} />,
      sortValue: (row) => row.status ?? '',
    },
  ];

  return (
    <DataTable
      columns={columns}
      rows={sensors}
      rowKey={(row) => row.id}
      onRowClick={(row) => navigate(`/sensors/${row.id}`)}
      initialSort={{ columnId: 'sensor_id', direction: 'asc' }}
      emptyState={
        <EmptyState
          title="No sensors yet"
          description="Sensors are AI perception entities — they observe a source or composer, run a detector, and emit findings. Create one, then a composer input can pick it for AI auto-crop."
        />
      }
    />
  );
}
