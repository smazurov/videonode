import React from "react";
import { cn } from "../../utils";

export interface KVEntry {
  readonly label: React.ReactNode;
  readonly value: React.ReactNode;
  readonly hint?: React.ReactNode;
  readonly mono?: boolean;
}

export interface KVInspectorProps {
  readonly entries: readonly KVEntry[];
  readonly dense?: boolean;
  readonly className?: string;
  readonly emptyText?: React.ReactNode;
}

export function KVInspector({
  entries,
  dense = false,
  className,
}: Readonly<KVInspectorProps>) {
  return (
    <dl
      className={cn(
        "grid grid-cols-[max-content_1fr] gap-x-4 text-sm",
        dense ? "gap-y-1" : "gap-y-2",
        className,
      )}
    >
      {entries.map((entry, idx) => (
        <React.Fragment key={idx}>
          <dt className={cn("text-fg-muted font-medium", dense ? "py-0.5" : "py-1")}>
            {entry.label}
          </dt>
          <dd className={cn("text-fg min-w-0 break-words", dense ? "py-0.5" : "py-1", entry.mono ? "font-mono" : undefined)}>
            <div>{entry.value}</div>
            {entry.hint && (
              <div className="text-xs text-fg-subtle mt-0.5">{entry.hint}</div>
            )}
          </dd>
        </React.Fragment>
      ))}
    </dl>
  );
}
