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
    </dl>
  );
}
