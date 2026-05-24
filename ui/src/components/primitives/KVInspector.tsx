<<<<<<< HEAD
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
=======
import type { ReactNode } from 'react';
import { cn } from '../../utils';

export interface KVEntry {
  readonly label: ReactNode;
  readonly value: ReactNode;
  readonly mono?: boolean;
}

interface KVInspectorProps {
  readonly entries: ReadonlyArray<KVEntry>;
  readonly className?: string;
  readonly dense?: boolean;
}

export function KVInspector({ entries, className, dense }: KVInspectorProps) {
  return (
    <dl className={cn('divide-y divide-border', className)}>
      {entries.map((entry, idx) => (
        <div
          key={typeof entry.label === 'string' ? entry.label : idx}
          className={cn('flex items-baseline justify-between gap-3', dense ? 'py-1.5' : 'py-2')}
        >
          <dt className="text-xs uppercase tracking-wide text-fg-muted">{entry.label}</dt>
          <dd
            className={cn(
              'min-w-0 truncate text-right text-sm text-fg',
              entry.mono ? 'font-mono' : undefined,
            )}
          >
            {entry.value}
          </dd>
        </div>
      ))}
>>>>>>> worktree-agent-a00537f30a6b3ed35
    </dl>
  );
}
