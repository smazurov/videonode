import { ReactNode, useMemo, useState } from "react";
import { cn } from "../../utils";

// Stub for U5 — minimal sortable table; U5 may layer in selection, filtering.
export interface DataTableColumn<T> {
  key: string;
  header: ReactNode;
  cell: (row: T) => ReactNode;
  sortValue?: (row: T) => string | number;
  width?: string;
  className?: string;
}

interface DataTableProps<T> {
  columns: DataTableColumn<T>[];
  rows: T[];
  rowKey: (row: T) => string;
  onRowClick?: (row: T) => void;
  empty?: ReactNode;
  className?: string;
  initialSort?: { key: string; direction: "asc" | "desc" };
}

type SortState = { key: string; direction: "asc" | "desc" } | null;

export function DataTable<T>({
  columns,
  rows,
  rowKey,
  onRowClick,
  empty,
  className,
  initialSort,
}: Readonly<DataTableProps<T>>) {
  const [sort, setSort] = useState<SortState>(initialSort ?? null);

  const sortedRows = useMemo(() => {
    if (!sort) return rows;
    const col = columns.find((c) => c.key === sort.key);
    if (!col?.sortValue) return rows;
    const sorter = col.sortValue;
    return [...rows].sort((a, b) => {
      const av = sorter(a);
      const bv = sorter(b);
      if (av < bv) return sort.direction === "asc" ? -1 : 1;
      if (av > bv) return sort.direction === "asc" ? 1 : -1;
      return 0;
    });
  }, [rows, sort, columns]);

  const toggleSort = (key: string) => {
    setSort((prev) => {
      if (!prev || prev.key !== key) return { key, direction: "asc" };
      if (prev.direction === "asc") return { key, direction: "desc" };
      return null;
    });
  };

  if (rows.length === 0 && empty) {
    return <div className={className}>{empty}</div>;
  }

  return (
    <div className={cn("overflow-x-auto rounded-md border border-border bg-surface-raised", className)}>
      <table className="w-full text-sm">
        <thead className="border-b border-border bg-surface-muted/50">
          <tr>
            {columns.map((col) => {
              const sortable = !!col.sortValue;
              const active = sort?.key === col.key;
              return (
                <th
                  key={col.key}
                  scope="col"
                  className={cn(
                    "text-left font-medium text-fg-muted px-4 py-2",
                    sortable ? "cursor-pointer select-none hover:text-fg" : "",
                    col.className,
                  )}
                  {...(col.width ? { style: { width: col.width } } : {})}
                  {...(sortable ? { onClick: () => toggleSort(col.key) } : {})}
                >
                  <span className="inline-flex items-center gap-1">
                    {col.header}
                    {sortable && active && (
                      <span aria-hidden className="text-xs">
                        {sort.direction === "asc" ? "▲" : "▼"}
                      </span>
                    )}
                  </span>
                </th>
              );
            })}
          </tr>
        </thead>
        <tbody>
          {sortedRows.map((row) => (
            <tr
              key={rowKey(row)}
              className={cn(
                "border-b border-border last:border-b-0",
                onRowClick ? "cursor-pointer hover:bg-surface-muted/40" : "",
              )}
              {...(onRowClick ? { onClick: () => onRowClick(row) } : {})}
            >
              {columns.map((col) => (
                <td key={col.key} className={cn("px-4 py-2 text-fg align-middle", col.className)}>
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
