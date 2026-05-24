import { useEffect, useState } from 'react';
import type { components } from '../lib/api.generated';
import { api, unwrap } from '../lib/api';

export type ProcessEntry = components['schemas']['ProcessEntry'];

interface UseProcessesOptions {
  enabled?: boolean;
  intervalMs?: number;
}

interface UseProcessesResult {
  processes: ProcessEntry[];
  loading: boolean;
  error: string | null;
}

export function useProcesses({
  enabled = true,
  intervalMs = 2000,
}: UseProcessesOptions = {}): UseProcessesResult {
  const [processes, setProcesses] = useState<ProcessEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!enabled) return;

    let cancelled = false;
    let timer: number | null = null;

    const fetchOnce = async () => {
      const controller = new AbortController();
      try {
        if (processes.length === 0) setLoading(true);
        const data = unwrap(
          await api.GET('/api/processes', { signal: controller.signal }),
          'Failed to load processes',
        );
        if (cancelled) return;
        setProcesses(data.processes ?? []);
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
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [enabled, intervalMs]);

  return { processes, loading, error };
}
