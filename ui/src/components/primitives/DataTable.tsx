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
  readonly getRowId?: (row: T) => string;
  readonly rowKey?: (row: T) => string; // alias for getRowId (back-compat)
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
      return null;
    });
  };

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
              return (
                <th
                  key={col.id}
                  scope="col"
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
                  </span>
                </th>
              );
            })}
          </tr>
        </thead>
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
        </tbody>
      </table>
    </div>
  );
}
