import { useMemo } from 'react';
import { Badge } from '../Badge';
import { useProcesses, type ProcessEntry } from '../../hooks/useProcesses';
import { formatUptime as formatUptimeShared } from '../../lib/formatUptime';

const STATE_TONE: Record<string, 'success' | 'warning' | 'danger' | 'neutral'> = {
  running: 'success',
  starting: 'warning',
  stopping: 'warning',
  idle: 'neutral',
  error: 'danger',
};

const KIND_TONE: Record<string, 'canvas' | 'webrtc' | 'rtsp' | 'neutral'> = {
  producer: 'rtsp',
  composer: 'canvas',
  encoder: 'webrtc',
};

function formatUptime(startedAtUS?: number): string {
  return formatUptimeShared(startedAtUS) ?? '—';
}

function formatRSS(bytes: number): string {
  if (bytes >= 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`;
  if (bytes >= 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  if (bytes >= 1024) return `${(bytes / 1024).toFixed(0)} KB`;
  return `${bytes} B`;
}

interface ProcessRowProps {
  readonly proc: ProcessEntry;
}

function ProcessRow({ proc }: ProcessRowProps) {
  const stateTone = STATE_TONE[proc.state] ?? 'neutral';
  const kindTone = KIND_TONE[proc.kind] ?? 'neutral';
  const isError = proc.state === 'error';

  return (
    <div
      className={`px-3 py-2 border-b border-border text-sm ${
        isError ? 'bg-danger-soft/30' : 'hover:bg-surface-muted'
      }`}
    >
      <div className="flex items-center gap-2 mb-1">
        <Badge tone={kindTone} size="xs">{proc.kind}</Badge>
        <Badge tone={stateTone} size="xs">{proc.state}</Badge>
        {proc.restart_count !== undefined && proc.restart_count > 0 && (
          <Badge tone="warning" size="xs" title="Restart count">
            ↻ {proc.restart_count}
          </Badge>
        )}
      </div>
      <div className="font-mono text-xs text-fg break-all" title={proc.id}>
        {proc.id}
      </div>
      <div className="mt-1 grid grid-cols-2 gap-x-2 gap-y-0.5 text-xs text-fg-muted font-mono">
        {proc.stream_id && (
          <>
            <span className="text-fg-subtle">stream</span>
            <span className="text-fg">{proc.stream_id}</span>
          </>
        )}
        {proc.pid !== undefined && proc.pid > 0 && (
          <>
            <span className="text-fg-subtle">pid</span>
            <span className="text-fg">{proc.pid}</span>
          </>
        )}
        {proc.started_at_us !== undefined && proc.started_at_us > 0 && (
          <>
            <span className="text-fg-subtle">uptime</span>
            <span className="text-fg">{formatUptime(proc.started_at_us)}</span>
          </>
        )}
        {proc.cpu_percent !== undefined && proc.cpu_percent > 0 && (
          <>
            <span className="text-fg-subtle">cpu</span>
            <span className="text-fg">{proc.cpu_percent.toFixed(1)}%</span>
          </>
        )}
        {proc.rss_bytes !== undefined && proc.rss_bytes > 0 && (
          <>
            <span className="text-fg-subtle">rss</span>
            <span className="text-fg">{formatRSS(proc.rss_bytes)}</span>
          </>
        )}
        {proc.refcount !== undefined && proc.refcount > 0 && (
          <>
            <span className="text-fg-subtle">refcount</span>
            <span className="text-fg">{proc.refcount}</span>
          </>
        )}
      </div>
      {proc.consumers && proc.consumers.length > 0 && (
        <div className="mt-1 text-xs text-fg-muted">
          <span className="text-fg-subtle">consumers: </span>
          <span className="text-fg font-mono">{proc.consumers.join(', ')}</span>
        </div>
      )}
      {proc.last_error && (
        <div className="mt-1 text-xs text-danger font-mono break-all" title={proc.last_error}>
          {proc.last_error}
        </div>
      )}
    </div>
  );
}

interface ProcessListProps {
  readonly enabled?: boolean;
}

export function ProcessList({ enabled = true }: ProcessListProps) {
  const { processes, loading, error } = useProcesses({ enabled });

  const grouped = useMemo(() => {
    const order: Record<string, number> = { producer: 0, composer: 1, encoder: 2 };
    return [...processes].sort((a, b) => {
      const ka = order[a.kind] ?? 99;
      const kb = order[b.kind] ?? 99;
      if (ka !== kb) return ka - kb;
      return a.id.localeCompare(b.id);
    });
  }, [processes]);

  return (
    <div className="flex flex-col h-full bg-surface border-l border-border">
      <div className="px-3 py-2 border-b border-border shrink-0 flex items-center justify-between">
        <div className="text-sm font-medium text-fg">Processes</div>
        <div className="text-xs text-fg-subtle tabular-nums">
          {processes.length}
        </div>
      </div>
      {error && (
        <div className="px-3 py-2 text-xs text-danger border-b border-border">
          {error}
        </div>
      )}
      <div className="flex-1 overflow-auto min-h-0">
        {grouped.length === 0 && !loading && !error && (
          <div className="px-3 py-4 text-sm text-fg-subtle">No processes running</div>
        )}
        {grouped.map((proc) => (
          <ProcessRow key={proc.id} proc={proc} />
        ))}
      </div>
    </div>
  );
}
