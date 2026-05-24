import { useNavigate } from 'react-router-dom';
import { Badge } from '../Badge';
import { DataTable, type DataTableColumn } from '../primitives/DataTable';
import type { ComposerData, ComposerStatus } from '../../lib/composer-types';
import { formatCanvasDims } from '../../lib/composer-types';

interface ComposerListProps {
  composers: ComposerData[];
}

function statusTone(status: ComposerStatus | undefined): React.ComponentProps<typeof Badge>['tone'] {
  switch (status) {
    case 'warm':
      return 'success';
    case 'cold':
      return 'neutral';
    case 'error':
      return 'danger';
    case 'idle':
      return 'info';
    default:
      return 'neutral';
  }
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
      cell: (row) => (
        <Badge tone={statusTone(row.status)} size="xs">
          {row.status ?? 'unknown'}
        </Badge>
      ),
    },
  ];

  return (
    <DataTable
      columns={columns}
      rows={composers}
      getRowId={(row) => row.composer_id}
      onRowClick={(row) => navigate(`/composers/${encodeURIComponent(row.composer_id)}`)}
      empty="No composers defined yet."
    />
  );
}
