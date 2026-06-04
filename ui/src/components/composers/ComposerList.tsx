import { useNavigate } from 'react-router-dom';
import { DataTable, type DataTableColumn } from '../primitives/DataTable';
import { StatusPill } from '../primitives/StatusPill';
import { poolStateToPill } from '../../lib/pool-status';
import type { ComposerData } from '../../lib/composer-types';
import { formatCanvasDims } from '../../lib/composer-types';

interface ComposerListProps {
  composers: ComposerData[];
}

export function ComposerList({ composers }: Readonly<ComposerListProps>) {
  const navigate = useNavigate();

  const columns: DataTableColumn<ComposerData>[] = [
    {
      id: 'composer-id',
      header: 'Composer',
      cell: (row) => <span className="font-medium">{row.composer_id}</span>,
    },
    {
      id: 'canvas',
      header: 'Canvas',
      cell: (row) => (
        <span className="font-mono text-xs text-fg-muted">{formatCanvasDims(row.canvas)}</span>
      ),
    },
    {
      id: 'inputs',
      header: 'Inputs',
      cell: (row) => (
        <span className="tabular-nums">{row.inputs.length}</span>
      ),
    },
    {
      id: 'downstream',
      header: 'Streams',
      cell: (row) => (
        <span className="tabular-nums">{row.downstream_stream_ids?.length ?? 0}</span>
      ),
    },
    {
      id: 'status',
      header: 'Status',
      cell: (row) => <StatusPill status={poolStateToPill(row.status)} size="xs" />,
    },
  ];

  return (
    <DataTable<ComposerData>
      columns={columns}
      rows={composers}
      rowKey={(row) => row.composer_id}
      onRowClick={(row) => navigate(`/composers/${encodeURIComponent(row.composer_id)}`)}
      emptyState="No composers defined yet."
    />
  );
}
