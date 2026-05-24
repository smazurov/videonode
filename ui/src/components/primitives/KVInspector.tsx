import React from "react";
import { cn } from "../../utils";

export interface KVItem {
  readonly label?: React.ReactNode;
  readonly value: React.ReactNode;
  readonly hint?: React.ReactNode;
  readonly key?: string; // doubles as label when label is omitted (back-compat)
  readonly mono?: boolean; // render value in monospace font (back-compat)
}

// KVEntry is an alias for KVItem retained for back-compat with earlier
// consumers that imported the name directly.
export type KVEntry = KVItem;

export interface KVInspectorProps {
  readonly items?: readonly KVItem[];
  readonly entries?: readonly KVItem[]; // alias for items (back-compat)
  readonly dense?: boolean;
  readonly className?: string;
  readonly emptyText?: React.ReactNode;
}

export function KVInspector({
  items,
  entries,
  dense = false,
  className,
}: Readonly<KVInspectorProps>) {
  const rows: readonly KVItem[] = items ?? entries ?? [];
  return (
    <dl
      className={cn(
        "grid grid-cols-[max-content_1fr] gap-x-4 text-sm",
        dense ? "gap-y-1" : "gap-y-2",
        className,
      )}
    >
      {rows.map((item, idx) => {
        const rowKey = item.key ?? `${idx}`;
        return (
          <React.Fragment key={rowKey}>
            <dt className={cn("text-fg-muted font-medium", dense ? "py-0.5" : "py-1")}>
              {item.label}
            </dt>
            <dd className={cn("text-fg min-w-0 break-words", dense ? "py-0.5" : "py-1")}>
              <div>{item.value}</div>
              {item.hint && (
                <div className="text-xs text-fg-subtle mt-0.5">{item.hint}</div>
              )}
            </dd>
          </React.Fragment>
        );
      })}
    </dl>
  );
}
