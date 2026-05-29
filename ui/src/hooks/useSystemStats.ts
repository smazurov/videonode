import { useEffect, useRef, useState } from 'react';
import type { components } from '../lib/api.generated';
import { api, unwrap } from '../lib/api';

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

  useEffect(() => {
    if (!enabled) return;

    let cancelled = false;
    let timer: number | null = null;

    const fetchOnce = async () => {
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
        if (!cancelled) setLoading(false);
      }
    };

    void fetchOnce();
    timer = window.setInterval(fetchOnce, intervalMs);

    return () => {
      cancelled = true;
      if (timer != null) window.clearInterval(timer);
    };
  }, [enabled, intervalMs]);

  return { stats, loading, error };
}
