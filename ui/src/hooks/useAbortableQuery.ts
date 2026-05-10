import { useState, useEffect, type DependencyList } from 'react';

export interface AbortableQueryResult<T> {
  data: T;
  loading: boolean;
  error: string | null;
}

export interface AbortableQueryOptions<T> {
  initial: T;
  enabled?: boolean;
  onError?: (err: Error) => string | null;
}

// useAbortableQuery runs an async fetcher whenever deps change, aborting the
// previous request first. Returns { data, loading, error } with the same
// stable shape across all callers. Set enabled=false to skip the fetch and
// reset to initial.
export function useAbortableQuery<T>(
  fetcher: (signal: AbortSignal) => Promise<T>,
  deps: DependencyList,
  opts: AbortableQueryOptions<T>,
): AbortableQueryResult<T> {
  const { initial, enabled = true, onError } = opts;
  const [data, setData] = useState<T>(initial);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!enabled) {
      setData(initial);
      setLoading(false);
      setError(null);
      return;
    }

    const controller = new AbortController();
    const signal = controller.signal;

    setData(initial);
    setLoading(true);
    setError(null);

    fetcher(signal)
      .then((result) => {
        if (signal.aborted) return;
        setData(result);
        setError(null);
      })
      .catch((error_: unknown) => {
        if (signal.aborted) return;
        const e = error_ instanceof Error ? error_ : new Error(String(error_));
        if (e.name === 'AbortError') return;
        const handled = onError ? onError(e) : e.message;
        setError(handled);
        setData(initial);
      })
      .finally(() => {
        if (!signal.aborted) setLoading(false);
      });

    return () => {
      controller.abort();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);

  return { data, loading, error };
}
