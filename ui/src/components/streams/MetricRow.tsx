import { useLayoutEffect, useRef, useState } from 'react';
import { InformationCircleIcon } from '@heroicons/react/24/outline';
import { Sparkline } from '../primitives/Sparkline';
import { cn } from '../../utils';

// Each sample is rendered as a fixed-width step; narrower containers show fewer
// (newest) samples rather than squeezing the steps.
const STEP_PX = 12;
const ROW_HEIGHT = 48;

function lastNumber(values: ReadonlyArray<number | null>): number | null {
  for (let i = values.length - 1; i >= 0; i -= 1) {
    const v = values[i];
    if (v != null) return v;
  }
  return null;
}

interface MetricRowProps {
  readonly label: string;
  readonly values: ReadonlyArray<number | null>;
  readonly formatValue: (n: number) => string;
  readonly tone: 'success' | 'accent';
  readonly stale?: boolean;
  readonly current?: number | null | undefined;
  readonly title?: string | undefined;
  /** Share of the row the chart may cover (from the right); below 1 keeps it clear of the label. */
  readonly chartFraction?: number;
}

export function MetricRow({ label, values, formatValue, tone, stale = false, current, title, chartFraction = 1 }: MetricRowProps) {
  const ref = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(0);

  // Measure once on mount so the chart draws without waiting for the first
  // ResizeObserver callback, then track resizes.
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return undefined;
    setWidth(el.getBoundingClientRect().width);
    const observer = new ResizeObserver((entries) => {
      setWidth(entries[0]?.contentRect.width ?? 0);
    });
    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  const visibleCount = width > 0 ? Math.min(values.length, Math.floor((width * chartFraction) / STEP_PX)) : 0;
  const visible = values.slice(values.length - visibleCount);
  const latest = current === undefined ? lastNumber(values) : current;
  const toneClass = tone === 'success' ? 'text-success' : 'text-accent';

  return (
    <div
      ref={ref}
      className={cn('relative overflow-hidden rounded-md', title && 'cursor-help')}
      style={{ height: ROW_HEIGHT }}
      title={title}
    >
      {visibleCount > 0 && (
        <Sparkline
          values={visible}
          width={visible.length * STEP_PX}
          height={ROW_HEIGHT}
          className={cn('absolute inset-y-0 right-0', toneClass)}
        />
      )}
      <div className="relative z-10 flex flex-col px-2 py-1.5">
        <span className="flex items-center gap-1 text-xs uppercase tracking-wide text-fg-muted">
          {label}
          {title && <InformationCircleIcon aria-hidden className="h-3.5 w-3.5 text-fg-subtle" />}
        </span>
        <span className={cn('font-mono text-sm', stale ? 'text-fg-subtle' : 'text-fg')}>
          {latest != null ? formatValue(latest) : '—'}
          {stale && latest != null && <span className="ml-2 text-xs">last reported · stale</span>}
        </span>
      </div>
    </div>
  );
}
