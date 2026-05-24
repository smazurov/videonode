import { ReactNode } from 'react';
import { cn } from '../../utils';

export interface DataTableColumn<T> {
  id: string;
  header: ReactNode;
  cell: (row: T) => ReactNode;
  className?: string;
}

interface DataTableProps<T> {
  columns: DataTableColumn<T>[];
  rows: T[];
  getRowId: (row: T) => string;
  onRowClick?: (row: T) => void;
  empty?: ReactNode;
  className?: string;
}

// Minimal sortable-by-click-not-yet DataTable. U5 owns the canonical
// primitive; we ship enough here to render the composers list page.
export function DataTable<T>({
  columns,
  rows,
  getRowId,
  onRowClick,
  empty,
  className,
}: Readonly<DataTableProps<T>>) {
  if (rows.length === 0) {
    return (
      <div className="rounded-lg border border-border bg-surface-raised p-8 text-center text-sm text-fg-muted">
        {empty ?? 'No rows.'}
      </div>
    );
  }

  return (
    <div className={cn('overflow-hidden rounded-lg border border-border bg-surface-raised', className)}>
      <table className="w-full border-collapse text-sm">
        <thead className="bg-surface-sunken">
          <tr>
            {columns.map((col) => (
              <th
                key={col.id}
                scope="col"
                className={cn(
                  'px-4 py-2 text-left text-xs font-medium uppercase tracking-wide text-fg-muted',
                  col.className,
                )}
              >
                {col.header}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => {
            const rowId = getRowId(row);
            const clickable = !!onRowClick;
            return (
              <tr
                key={rowId}
                className={cn(
                  'border-t border-border',
                  clickable ? 'cursor-pointer hover:bg-surface-muted' : undefined,
                )}
                onClick={clickable ? () => onRowClick(row) : undefined}
                onKeyDown={
                  clickable
                    ? (e) => {
                        if (e.key === 'Enter' || e.key === ' ') {
                          e.preventDefault();
                          onRowClick(row);
                        }
                      }
                    : undefined
                }
                tabIndex={clickable ? 0 : undefined}
                role={clickable ? 'button' : undefined}
              >
                {columns.map((col) => (
                  <td key={col.id} className={cn('px-4 py-3 align-middle text-fg', col.className)}>
                    {col.cell(row)}
                  </td>
                ))}
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
