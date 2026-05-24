<<<<<<< HEAD
import React, { useMemo, useState } from "react";
import { ChevronUpIcon, ChevronDownIcon } from "@heroicons/react/24/outline";
import { cva, cn, type VariantProps } from "../../utils";

export type SortDirection = "asc" | "desc";

export interface DataTableColumn<T> {
  readonly id: string;
  readonly label: React.ReactNode;
  readonly accessor: (row: T) => React.ReactNode;
  /** Optional custom comparator. Required to mark a column sortable when accessor isn't a primitive. */
  readonly sort?: (a: T, b: T) => number;
  /** Whether this column is sortable (defaults to true if sort is provided). */
  readonly sortable?: boolean;
  readonly align?: "left" | "right" | "center";
  readonly className?: string;
  readonly headerClassName?: string;
  readonly width?: string;
}

const tableVariants = cva({
  base: "w-full text-sm border-collapse",
  variants: {
    density: {
      compact: "[&_td]:py-1.5 [&_th]:py-1.5",
      regular: "[&_td]:py-2 [&_th]:py-2",
    },
  },
  defaultVariants: { density: "regular" },
});

export interface DataTableProps<T> extends VariantProps<typeof tableVariants> {
  readonly columns: ReadonlyArray<DataTableColumn<T>>;
  readonly rows: readonly T[];
  readonly getRowId: (row: T) => string;
  readonly onRowClick?: (row: T) => void;
  /** Controlled multi-select state. Pass `undefined` to disable selection. */
  readonly selection?: ReadonlyArray<string>;
  readonly onSelectionChange?: (next: string[]) => void;
  readonly filter?: string;
  readonly onFilterChange?: (next: string) => void;
  readonly emptyState?: React.ReactNode;
  readonly className?: string;
  readonly initialSort?: { columnId: string; direction: SortDirection };
}

function defaultRowMatch(row: unknown, needle: string): boolean {
  if (row == null) return false;
  if (typeof row === "string" || typeof row === "number" || typeof row === "boolean") {
    return String(row).toLowerCase().includes(needle);
  }
  if (typeof row === "object") {
    return Object.values(row as Record<string, unknown>).some((v) =>
      defaultRowMatch(v, needle),
    );
  }
  return false;
}

const alignClass = (align: DataTableColumn<unknown>["align"]): string => {
  switch (align) {
    case "right":
      return "text-right";
    case "center":
      return "text-center";
    default:
      return "text-left";
  }
};

export function DataTable<T>({
  columns,
  rows,
  getRowId,
  onRowClick,
  selection,
  onSelectionChange,
  filter,
  emptyState,
  className,
  density,
  initialSort,
}: Readonly<DataTableProps<T>>) {
  const [sortState, setSortState] = useState<{ columnId: string; direction: SortDirection } | null>(
    initialSort ?? null,
  );

  const selectionEnabled = selection !== undefined;
  const selectedSet = useMemo(() => new Set(selection ?? []), [selection]);

  const filteredRows = useMemo(() => {
    if (!filter) return rows;
    const needle = filter.toLowerCase().trim();
    if (!needle) return rows;
    return rows.filter((row) => defaultRowMatch(row, needle));
  }, [rows, filter]);

  const sortedRows = useMemo(() => {
    if (!sortState) return filteredRows;
    const col = columns.find((c) => c.id === sortState.columnId);
    if (!col?.sort) return filteredRows;
    const dir = sortState.direction === "asc" ? 1 : -1;
    const cmp = col.sort;
    return [...filteredRows].sort((a, b) => cmp(a, b) * dir);
  }, [filteredRows, sortState, columns]);

  const toggleSort = (col: DataTableColumn<T>) => {
    const sortable = col.sortable ?? Boolean(col.sort);
    if (!sortable || !col.sort) return;
    setSortState((prev) => {
      if (!prev || prev.columnId !== col.id) {
        return { columnId: col.id, direction: "asc" };
      }
      if (prev.direction === "asc") {
        return { columnId: col.id, direction: "desc" };
      }
=======
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
>>>>>>> worktree-agent-a00537f30a6b3ed35
      return null;
    });
  };

<<<<<<< HEAD
  const allVisibleIds = useMemo(() => sortedRows.map((r) => getRowId(r)), [sortedRows, getRowId]);
  const allSelected = selectionEnabled && allVisibleIds.length > 0
    && allVisibleIds.every((id) => selectedSet.has(id));
  const someSelected = selectionEnabled
    && !allSelected
    && allVisibleIds.some((id) => selectedSet.has(id));

  const headerCheckboxRef = React.useRef<HTMLInputElement | null>(null);
  React.useEffect(() => {
    if (headerCheckboxRef.current) {
      headerCheckboxRef.current.indeterminate = someSelected;
    }
  }, [someSelected]);

  const toggleAll = () => {
    if (!selectionEnabled || !onSelectionChange) return;
    if (allSelected) {
      onSelectionChange((selection ?? []).filter((id) => !allVisibleIds.includes(id)));
    } else {
      const next = new Set([...(selection ?? []), ...allVisibleIds]);
      onSelectionChange([...next]);
    }
  };

  const toggleRow = (id: string) => {
    if (!selectionEnabled || !onSelectionChange) return;
    const next = new Set(selection ?? []);
    if (next.has(id)) {
      next.delete(id);
    } else {
      next.add(id);
    }
    onSelectionChange([...next]);
  };

  return (
    <div className={cn("w-full overflow-x-auto rounded-lg border border-border bg-surface", className)}>
      <table className={tableVariants({ density })}>
        <thead className="bg-surface-muted text-fg-muted text-xs uppercase tracking-wide">
          <tr>
            {selectionEnabled && (
              <th className="w-10 px-3">
                <input
                  ref={headerCheckboxRef}
                  type="checkbox"
                  aria-label="Select all rows"
                  checked={allSelected}
                  onChange={toggleAll}
                  className="rounded border-border-strong text-accent focus:ring-focus-ring"
                />
              </th>
            )}
            {columns.map((col) => {
              const sortable = (col.sortable ?? Boolean(col.sort)) && Boolean(col.sort);
              const active = sortState?.columnId === col.id;
=======
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
>>>>>>> worktree-agent-a00537f30a6b3ed35
              return (
                <th
                  key={col.id}
                  scope="col"
<<<<<<< HEAD
                  className={cn(
                    "px-3 font-semibold",
                    alignClass(col.align),
                    sortable ? "cursor-pointer select-none hover:text-fg" : undefined,
                    col.headerClassName,
                  )}
                  style={col.width ? { width: col.width } : undefined}
                  onClick={() => toggleSort(col)}
                >
                  <span className="inline-flex items-center gap-1">
                    {col.label}
                    {sortable && active && (
                      sortState?.direction === "asc"
                        ? <ChevronUpIcon className="h-3 w-3" />
                        : <ChevronDownIcon className="h-3 w-3" />
                    )}
=======
                  className={cn('px-3 py-2 font-medium', sortable ? 'cursor-pointer select-none' : undefined, col.headerClassName)}
                  onClick={sortable ? () => toggleSort(col.id) : undefined}
                >
                  <span className="inline-flex items-center gap-1">
                    {col.header}
                    {sortable && active && sort?.direction === 'asc' && <ChevronUpIcon className="h-3 w-3" />}
                    {sortable && active && sort?.direction === 'desc' && <ChevronDownIcon className="h-3 w-3" />}
>>>>>>> worktree-agent-a00537f30a6b3ed35
                  </span>
                </th>
              );
            })}
          </tr>
        </thead>
<<<<<<< HEAD
        <tbody>
          {sortedRows.length === 0 ? (
            <tr>
              <td
                colSpan={columns.length + (selectionEnabled ? 1 : 0)}
                className="text-center text-fg-muted py-8"
              >
                {emptyState ?? "No rows"}
              </td>
            </tr>
          ) : (
            sortedRows.map((row) => {
              const id = getRowId(row);
              const isSelected = selectedSet.has(id);
              const clickable = Boolean(onRowClick);
              return (
                <tr
                  key={id}
                  className={cn(
                    "border-t border-border",
                    clickable ? "cursor-pointer hover:bg-surface-muted" : undefined,
                    isSelected ? "bg-accent-soft/50" : undefined,
                  )}
                  onClick={onRowClick ? () => onRowClick(row) : undefined}
                >
                  {selectionEnabled && (
                    <td className="px-3" onClick={(e) => e.stopPropagation()}>
                      <input
                        type="checkbox"
                        aria-label={`Select row ${id}`}
                        checked={isSelected}
                        onChange={() => toggleRow(id)}
                        className="rounded border-border-strong text-accent focus:ring-focus-ring"
                      />
                    </td>
                  )}
                  {columns.map((col) => (
                    <td
                      key={col.id}
                      className={cn("px-3 align-middle text-fg", alignClass(col.align), col.className)}
                    >
                      {col.accessor(row)}
                    </td>
                  ))}
                </tr>
              );
            })
          )}
=======
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
>>>>>>> worktree-agent-a00537f30a6b3ed35
        </tbody>
      </table>
    </div>
  );
}
