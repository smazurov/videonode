import { useEffect, useRef, useState } from 'react';
import type { components } from '../lib/api.generated';
import { api, unwrap } from '../lib/api';
import { useConnectionStatus } from './useConnectionStatus';

export type SystemStats = components['schemas']['SystemStatsResponseBody'];

interface UseSystemStatsOptions {
  enabled?: boolean;
  intervalMs?: number;
}

interface UseSystemStatsResult {
  stats: SystemStats | null;
  loading: boolean;
  error: string | null;
}

export function useSystemStats({
  enabled = true,
  intervalMs = 2000,
}: UseSystemStatsOptions = {}): UseSystemStatsResult {
  const [stats, setStats] = useState<SystemStats | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const hasDataRef = useRef(false);

  const online = useConnectionStatus() === 'online';

  useEffect(() => {
    // Rig offline: don't poll a dead host. The stale `stats` is masked at
    // return time (online ? stats : null) so the InfoBar shows '—' rather than
    // a frozen uptime; reset hasDataRef so the next online fetch shows loading.
    // Polling resumes when the connection returns (effect re-runs on `online`).
    if (!enabled || !online) {
      hasDataRef.current = false;
      return;
    }

    let cancelled = false;
    let fetching = false;
    let timer: number | null = null;

    const fetchOnce = async () => {
      if (fetching) return; // in-flight guard: skip the tick if one is pending
      fetching = true;
      try {
        if (!hasDataRef.current) setLoading(true);
        const data = unwrap(await api.GET('/api/system'), 'Failed to load system stats');
        if (cancelled) return;
        hasDataRef.current = true;
        setStats(data);
        setError(null);
      } catch (error_: unknown) {
        if (cancelled) return;
        const e = error_ instanceof Error ? error_ : new Error(String(error_));
        if (e.name === 'AbortError') return;
        setError(e.message);
      } finally {
        fetching = false;
        if (!cancelled) setLoading(false);
      }
    };

    void fetchOnce();
    timer = window.setInterval(fetchOnce, intervalMs);

    return () => {
      cancelled = true;
      if (timer != null) window.clearInterval(timer);
    };
  }, [enabled, intervalMs, online]);

  return { stats: online ? stats : null, loading, error };
}
