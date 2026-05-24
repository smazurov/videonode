import { useEffect, useRef, useState } from 'react';
import { useStreamStore } from '../../hooks/useStreamStore';
import { KVInspector, type KVEntry } from '../primitives/KVInspector';
import { SectionHeader } from '../primitives/SectionHeader';
import { cn } from '../../utils';

interface StreamMetricsPanelProps {
  readonly streamId: string;
  readonly className?: string;
}

const HISTORY_LENGTH = 60;

interface Sample {
  readonly fps: number | null;
}

function calculateUptime(startTime: string | undefined): string {
  if (!startTime) return 'N/A';
  try {
    const start = new Date(startTime);
    const now = new Date();
    const uptimeMs = now.getTime() - start.getTime();
    if (uptimeMs < 0) return 'N/A';
    const seconds = Math.floor(uptimeMs / 1000);
    const days = Math.floor(seconds / 86400);
    const hours = Math.floor((seconds % 86400) / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    const secs = seconds % 60;
    if (days > 0) return `${days}d ${hours}h ${minutes}m`;
    if (hours > 0) return `${hours}h ${minutes}m ${secs}s`;
    return `${minutes}m ${secs}s`;
  } catch {
    return 'N/A';
  }
}

function parseNumber(value: string | undefined): number | null {
  if (!value) return null;
  const n = Number.parseFloat(value);
  return Number.isFinite(n) ? n : null;
}

interface SparklineProps {
  readonly values: ReadonlyArray<number | null>;
  readonly label: string;
  readonly formatValue: (n: number) => string;
  readonly tone?: 'accent' | 'success' | 'warning';
}

function Sparkline({ values, label, formatValue, tone = 'accent' }: SparklineProps) {
  const numeric = values.filter((v): v is number => v != null);
  const max = numeric.length > 0 ? Math.max(...numeric, 1) : 1;
  const min = numeric.length > 0 ? Math.min(...numeric, 0) : 0;
  const range = Math.max(max - min, 1);
  const latest = numeric.length > 0 ? numeric[numeric.length - 1] : null;

  const width = 100;
  const height = 28;
  const step = values.length > 1 ? width / (values.length - 1) : width;

  const points = values
    .map((v, i) => {
      if (v == null) return null;
      const x = i * step;
      const y = height - ((v - min) / range) * height;
      return `${x.toFixed(2)},${y.toFixed(2)}`;
    })
    .filter((p): p is string => p !== null)
    .join(' ');

  const strokeClass = {
    accent: 'stroke-accent',
    success: 'stroke-success',
    warning: 'stroke-warning',
  }[tone];

  return (
    <div className="flex items-center justify-between gap-3">
      <div className="min-w-0">
        <div className="text-xs uppercase tracking-wide text-fg-muted">{label}</div>
        <div className="font-mono text-sm text-fg">
          {latest != null ? formatValue(latest) : '—'}
        </div>
      </div>
      <svg
        viewBox={`0 0 ${width} ${height}`}
        preserveAspectRatio="none"
        className={cn('h-7 w-32 shrink-0', strokeClass)}
        aria-hidden="true"
      >
        {points && (
          <polyline
            points={points}
            fill="none"
            strokeWidth="1.5"
            strokeLinejoin="round"
            strokeLinecap="round"
          />
        )}
      </svg>
    </div>
  );
}

export function StreamMetricsPanel({ streamId, className }: StreamMetricsPanelProps) {
  const metrics = useStreamStore((state) => state.metricsById[streamId]);
  const startTime = useStreamStore((state) => state.streamsById[streamId]?.created_at);
  const bitrate = useStreamStore((state) => state.streamsById[streamId]?.encoder?.bitrate);

  const [uptime, setUptime] = useState(() => calculateUptime(startTime));
  const [history, setHistory] = useState<Sample[]>([]);
  const lastFpsRef = useRef<string | undefined>(undefined);

  useEffect(() => {
    const interval = setInterval(() => setUptime(calculateUptime(startTime)), 1000);
    return () => clearInterval(interval);
  }, [startTime]);

  useEffect(() => {
    if (metrics?.fps === lastFpsRef.current) return;
    lastFpsRef.current = metrics?.fps;
    // Append the freshest sample to a bounded history ring. This is the
    // canonical "derived from external metric stream" pattern; React's
    // set-state-in-effect lint rule fires on it but the alternatives
    // (subscribeWithSelector, useSyncExternalStore on a parallel buffer)
    // are more invasive here.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setHistory((prev) => {
      const next = [...prev, { fps: parseNumber(metrics?.fps) }];
      if (next.length > HISTORY_LENGTH) next.shift();
      return next;
    });
  }, [metrics?.fps]);

  const fpsValues = history.map((h) => h.fps);

  const entries: KVEntry[] = [
    { label: 'Uptime', value: uptime, mono: true },
    { label: 'Bitrate', value: bitrate ?? '—', mono: true },
    {
      label: 'Dropped / Duplicate',
      value: `${metrics?.dropped_frames ?? '0'} / ${metrics?.duplicate_frames ?? '0'}`,
      mono: true,
    },
  ];

  return (
    <section className={cn('rounded-lg border border-border bg-surface-raised p-4', className)}>
      <SectionHeader title="Metrics" description="Live stream stats from the encoder" />
      <div className="mt-3 space-y-3">
        <Sparkline
          values={fpsValues}
          label="FPS"
          formatValue={(n) => `${n.toFixed(1)} fps`}
          tone="success"
        />
        <KVInspector entries={entries} dense />
      </div>
    </section>
  );
}
