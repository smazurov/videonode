import { useEffect, useMemo, useRef, useState } from 'react';
import { Area, AreaChart, XAxis, YAxis } from 'recharts';
import { useStreamStore } from '../../hooks/useStreamStore';
import { type MetricsSample } from '../../hooks/slices/streams/streamDataSlice';
import { useProcesses } from '../../hooks/useProcesses';
import { formatUptime } from '../../lib/formatUptime';
import { KVInspector, type KVEntry } from '../primitives/KVInspector';
import { SectionHeader } from '../primitives/SectionHeader';
import { cn } from '../../utils';

interface StreamMetricsPanelProps {
  readonly streamId: string;
  readonly className?: string;
}

const EMPTY_HISTORY: MetricsSample[] = [];

// Each sample is rendered as a fixed-width step; narrower containers show fewer
// (newest) samples rather than squeezing the steps.
const STEP_PX = 12;
const ROW_HEIGHT = 48;

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`;
}

function formatBitrate(bps: number): string {
  if (bps < 1000) return `${bps.toFixed(0)} bps`;
  if (bps < 1_000_000) return `${(bps / 1000).toFixed(0)} Kbps`;
  return `${(bps / 1_000_000).toFixed(2)} Mbps`;
}

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
}

function MetricRow({ label, values, formatValue, tone }: MetricRowProps) {
  const ref = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(0);

  useEffect(() => {
    const el = ref.current;
    if (!el) return undefined;
    const observer = new ResizeObserver((entries) => {
      setWidth(entries[0]?.contentRect.width ?? 0);
    });
    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  const visibleCount = width > 0 ? Math.min(values.length, Math.floor(width / STEP_PX)) : 0;
  const data = useMemo(
    () => values.slice(values.length - visibleCount).map((y, x) => ({ x, y })),
    [values, visibleCount],
  );
  const maxY = useMemo(() => {
    let m = 0;
    for (const d of data) if (d.y != null && d.y > m) m = d.y;
    return m > 0 ? m * 1.15 : 1;
  }, [data]);

  const latest = lastNumber(values);
  const toneClass = tone === 'success' ? 'text-success' : 'text-accent';

  return (
    <div
      ref={ref}
      className="relative overflow-hidden rounded-md"
      style={{ height: ROW_HEIGHT }}
    >
      {data.length > 0 && (
        <div
          className={cn('pointer-events-none absolute inset-y-0 right-0', toneClass)}
          aria-hidden="true"
        >
          <AreaChart
            width={data.length * STEP_PX}
            height={ROW_HEIGHT}
            data={data}
            margin={{ top: 4, right: 0, bottom: 0, left: 0 }}
          >
            <XAxis type="number" dataKey="x" domain={[0, Math.max(data.length - 1, 1)]} hide />
            <YAxis type="number" domain={[0, maxY]} hide />
            <Area
              type="step"
              dataKey="y"
              stroke="currentColor"
              strokeWidth={1}
              fill="currentColor"
              fillOpacity={0.16}
              isAnimationActive={false}
              connectNulls={false}
              dot={false}
              activeDot={false}
            />
          </AreaChart>
        </div>
      )}
      <div className="relative z-10 flex flex-col px-2 py-1.5">
        <span className="text-xs uppercase tracking-wide text-fg-muted">{label}</span>
        <span className="font-mono text-sm text-fg">
          {latest != null ? formatValue(latest) : '—'}
        </span>
      </div>
    </div>
  );
}

export function StreamMetricsPanel({ streamId, className }: StreamMetricsPanelProps) {
  const metrics = useStreamStore((state) => state.metricsById[streamId]);
  const metricsTs = useStreamStore((state) => state.metricsTsById[streamId]);
  const bitrate = useStreamStore((state) => state.streamsById[streamId]?.encoder?.bitrate);
  const history = useStreamStore((state) => state.metricsHistoryById[streamId]) ?? EMPTY_HISTORY;
  const { processes } = useProcesses({ enabled: true });

  const encoderStartUs = useMemo(() => {
    const encoder = processes.find((p) => p.kind === 'encoder' && p.stream_id === streamId);
    return encoder?.state === 'running' ? encoder.started_at_us : undefined;
  }, [processes, streamId]);

  const uptime = formatUptime(encoderStartUs, metricsTs) ?? '—';
  const fpsValues = useMemo(() => history.map((h) => h.fps), [history]);
  const bitrateValues = useMemo(() => history.map((h) => h.bitrateBps), [history]);

  const bytesOut = metrics?.bytes_out;
  const entries: KVEntry[] = [
    { label: 'Uptime', value: uptime, mono: true },
    { label: 'Bytes out', value: bytesOut != null ? formatBytes(bytesOut) : '—', mono: true },
    { label: 'Target bitrate', value: bitrate ?? '—', mono: true },
    {
      label: 'Dropped / Duplicate',
      value: `${metrics?.dropped_frames ?? '0'} / ${metrics?.duplicate_frames ?? '0'}`,
      mono: true,
    },
  ];

  return (
    <section className={cn('rounded-lg border border-border bg-surface-raised p-4', className)}>
      <SectionHeader title="Metrics" description="Live stream stats from the encoder" />
      <div className="mt-3 space-y-2">
        <MetricRow
          label="FPS"
          values={fpsValues}
          formatValue={(n) => `${n.toFixed(1)} fps`}
          tone="success"
        />
        <MetricRow
          label="Bitrate"
          values={bitrateValues}
          formatValue={formatBitrate}
          tone="accent"
        />
        <KVInspector entries={entries} dense />
      </div>
    </section>
  );
}
