import React from "react";
import { cn } from "../../utils";

export interface KVItem {
  readonly label: React.ReactNode;
  readonly value: React.ReactNode;
  readonly hint?: React.ReactNode;
  readonly key?: string;
}

export interface KVInspectorProps {
  readonly items: readonly KVItem[];
  readonly dense?: boolean;
  readonly className?: string;
}

export function KVInspector({
  items,
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
      {items.map((item, idx) => {
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
