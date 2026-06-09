import { useMemo } from 'react';
import { useProcesses, type ProcessEntry } from './useProcesses';
import { useConnectionStatus } from './useConnectionStatus';
import type { components } from '../lib/api.generated';

export type RKMPPCore = components['schemas']['RKMPPCore'];
export type DevfreqLoad = components['schemas']['DevfreqLoad'];

export interface SystemStatsError {
  id: string;
  message: string;
}

// Daemon-wide resource summary the InfoBar renders. Derived entirely from the
// `processes` SSE stream — the daemon is itself a row (id 'self', kind
// 'daemon') alongside the supervised stages, so the rollup is a pure
// client-side reduction with no dedicated endpoint.
export interface SystemStats {
  started_at_us: number;
  cpu_percent: number;
  rss_bytes: number;
  process_count: number;
  error_count: number;
  errors: SystemStatsError[];
  // Device-global hardware utilization, carried on the daemon ('self') row.
  // Undefined when the host lacks the hardware. Raw per-core data — the
  // InfoBar sums the RKMPP cores itself.
  rkmpp?: RKMPPCore[] | undefined;
  gpu?: DevfreqLoad | undefined;
}

interface UseSystemStatsOptions {
  enabled?: boolean;
}

interface UseSystemStatsResult {
  stats: SystemStats | null;
  loading: boolean;
  error: string | null;
}

// Reproduces the old /api/system math: footprint = the daemon plus every
// running supervised stage; errors = stages in the error state. The daemon's
// own row carries the uptime origin.
function reduce(processes: ProcessEntry[]): SystemStats {
  let cpu = 0;
  let rss = 0;
  let count = 0;
  let startedAtUs = 0;
  let rkmpp: RKMPPCore[] | undefined;
  let gpu: DevfreqLoad | undefined;
  const errors: SystemStatsError[] = [];
  for (const p of processes) {
    if (p.id === 'self') {
      startedAtUs = p.started_at_us ?? 0;
      rkmpp = p.rkmpp ?? undefined;
      gpu = p.gpu ?? undefined;
    }
    if (p.state === 'running' && (p.pid ?? 0) > 0) {
      cpu += p.cpu_percent ?? 0;
      rss += p.rss_bytes ?? 0;
      count += 1;
    }
    if (p.state === 'error') {
      errors.push({ id: p.id, message: p.last_error ?? '' });
    }
  }
  return {
    started_at_us: startedAtUs,
    cpu_percent: cpu,
    rss_bytes: rss,
    process_count: count,
    error_count: errors.length,
    errors,
    rkmpp,
    gpu,
  };
}

export function useSystemStats({
  enabled = true,
}: UseSystemStatsOptions = {}): UseSystemStatsResult {
  const { processes, loading, error } = useProcesses({ enabled });
  // Rig offline: blank the stats (return null) so the InfoBar shows '—' rather
  // than a frozen uptime ticking off a dead host. The processes stream goes
  // quiet on disconnect and resyncs on reconnect.
  const online = useConnectionStatus() === 'online';
  const stats = useMemo(() => (online ? reduce(processes) : null), [online, processes]);

  return { stats, loading, error };
}
