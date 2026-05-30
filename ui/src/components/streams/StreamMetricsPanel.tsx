import { useEffect, useMemo, useState } from 'react';
import { useStreamStore } from '../../hooks/useStreamStore';
import { useProcesses } from '../../hooks/useProcesses';
import { formatUptime } from '../../lib/formatUptime';
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

function parseNumber(value: string | number | undefined): number | null {
  if (value == null) return null;
  const n = typeof value === 'number' ? value : Number.parseFloat(value);
  return Number.isFinite(n) ? n : null;
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`;
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
  const bitrate = useStreamStore((state) => state.streamsById[streamId]?.encoder?.bitrate);
  const { processes } = useProcesses({ enabled: true });

  const encoderStartUs = useMemo(() => {
    const encoder = processes.find((p) => p.kind === 'encoder' && p.stream_id === streamId);
    return encoder?.state === 'running' ? encoder.started_at_us : undefined;
  }, [processes, streamId]);

  const uptime = formatUptime(encoderStartUs) ?? '—';
  const [history, setHistory] = useState<Sample[]>([]);

  useEffect(() => {
    let lastFps: number | undefined;
    return useStreamStore.subscribe((state) => {
      const fps = state.metricsById[streamId]?.fps;
      if (fps === lastFps) return;
      lastFps = fps;
      setHistory((prev) => {
        const next = [...prev, { fps: parseNumber(fps) }];
        if (next.length > HISTORY_LENGTH) next.shift();
        return next;
      });
    });
  }, [streamId]);

  const fpsValues = history.map((h) => h.fps);

  const bytesOut = parseNumber(metrics?.bytes_out);

  const entries: KVEntry[] = [
    { label: 'Uptime', value: uptime, mono: true },
    { label: 'Bitrate', value: bitrate ?? '—', mono: true },
    { label: 'Bytes out', value: bytesOut != null ? formatBytes(bytesOut) : '—', mono: true },
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
