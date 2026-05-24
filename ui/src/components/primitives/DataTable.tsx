import { useMemo, useState, type ReactNode } from 'react';
import { ChevronDownIcon, ChevronUpIcon } from '@heroicons/react/24/outline';
import { cn } from '../../utils';

export interface DataTableColumn<T> {
  id: string;
  header: ReactNode;
  cell: (row: T) => ReactNode;
  sortValue?: (row: T) => string | number;
  className?: string;
  headerClassName?: string;
}

export interface DataTableProps<T> {
  readonly rows: ReadonlyArray<T>;
  readonly columns: ReadonlyArray<DataTableColumn<T>>;
  readonly rowKey: (row: T) => string;
  readonly onRowClick?: (row: T) => void;
  readonly emptyState?: ReactNode;
  readonly initialSort?: { columnId: string; direction: 'asc' | 'desc' };
  readonly className?: string;
}

// Lightweight sortable table stub — U5 will replace with the full-featured
// version (filtering, selection). API kept stable so consumers don't churn.
export function DataTable<T>({
  rows,
  columns,
  rowKey,
  onRowClick,
  emptyState,
  initialSort,
  className,
}: DataTableProps<T>) {
  const [sort, setSort] = useState<{ columnId: string; direction: 'asc' | 'desc' } | null>(
    initialSort ?? null,
  );

  const sortedRows = useMemo(() => {
    if (!sort) return rows;
    const col = columns.find((c) => c.id === sort.columnId);
    if (!col?.sortValue) return rows;
    const factor = sort.direction === 'asc' ? 1 : -1;
    return [...rows].sort((a, b) => {
      const av = col.sortValue!(a);
      const bv = col.sortValue!(b);
      if (av < bv) return -1 * factor;
      if (av > bv) return 1 * factor;
      return 0;
    });
  }, [rows, sort, columns]);

  const toggleSort = (columnId: string) => {
    setSort((prev) => {
      if (prev?.columnId !== columnId) return { columnId, direction: 'asc' };
      if (prev.direction === 'asc') return { columnId, direction: 'desc' };
      return null;
    });
  };

  if (rows.length === 0 && emptyState) {
    return <div className={cn('rounded-lg border border-border bg-surface-raised', className)}>{emptyState}</div>;
  }

  return (
    <div className={cn('overflow-x-auto rounded-lg border border-border bg-surface-raised', className)}>
      <table className="min-w-full text-sm">
        <thead className="border-b border-border bg-surface-muted/40 text-left text-xs uppercase tracking-wide text-fg-muted">
          <tr>
            {columns.map((col) => {
              const sortable = !!col.sortValue;
              const active = sort?.columnId === col.id;
              return (
                <th
                  key={col.id}
                  scope="col"
                  className={cn('px-3 py-2 font-medium', sortable ? 'cursor-pointer select-none' : undefined, col.headerClassName)}
                  onClick={sortable ? () => toggleSort(col.id) : undefined}
                >
                  <span className="inline-flex items-center gap-1">
                    {col.header}
                    {sortable && active && sort?.direction === 'asc' && <ChevronUpIcon className="h-3 w-3" />}
                    {sortable && active && sort?.direction === 'desc' && <ChevronDownIcon className="h-3 w-3" />}
                  </span>
                </th>
              );
            })}
          </tr>
        </thead>
        <tbody className="divide-y divide-border">
          {sortedRows.map((row) => (
            <tr
              key={rowKey(row)}
              className={cn(
                'transition-colors',
                onRowClick ? 'cursor-pointer hover:bg-surface-muted/30' : undefined,
              )}
              onClick={onRowClick ? () => onRowClick(row) : undefined}
            >
              {columns.map((col) => (
                <td key={col.id} className={cn('px-3 py-2 align-middle', col.className)}>
                  {col.cell(row)}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
