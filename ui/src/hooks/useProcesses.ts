import { useEffect, useState } from 'react';
import type { components } from '../lib/api.generated';
import { api, unwrap } from '../lib/api';

export type ProcessEntry = components['schemas']['ProcessEntry'];

interface ProcessesState {
  processes: ProcessEntry[];
  loading: boolean;
  error: string | null;
}

const POLL_INTERVAL_MS = 2000;

// Shared, refcounted poller: every component that calls useProcesses reads
// the same `/api/processes` poll instead of starting its own timer. A
// stream-detail page mounts several panels that each need process state; one
// timer fans out to all of them.
let shared: ProcessesState = { processes: [], loading: false, error: null };
const subscribers = new Set<(s: ProcessesState) => void>();
let timer: number | null = null;

function emit() {
  for (const fn of subscribers) fn(shared);
}

async function fetchOnce() {
  const controller = new AbortController();
  try {
    if (shared.processes.length === 0 && !shared.loading) {
      shared = { ...shared, loading: true };
      emit();
    }
    const data = unwrap(
      await api.GET('/api/processes', { signal: controller.signal }),
      'Failed to load processes',
    );
    shared = { processes: data.processes ?? [], loading: false, error: null };
    emit();
  } catch (error_: unknown) {
    const e = error_ instanceof Error ? error_ : new Error(String(error_));
    if (e.name === 'AbortError') return;
    shared = { ...shared, loading: false, error: e.message };
    emit();
  }
}

function ensurePolling() {
  if (timer != null) return;
  void fetchOnce();
  timer = window.setInterval(() => void fetchOnce(), POLL_INTERVAL_MS);
}

function stopPolling() {
  if (timer != null) {
    window.clearInterval(timer);
    timer = null;
  }
}

interface UseProcessesOptions {
  enabled?: boolean;
}

interface UseProcessesResult {
  processes: ProcessEntry[];
  loading: boolean;
  error: string | null;
}

export function useProcesses({ enabled = true }: UseProcessesOptions = {}): UseProcessesResult {
  const [local, setLocal] = useState<ProcessesState>(shared);

  useEffect(() => {
    if (!enabled) return;
    const sub = (s: ProcessesState) => setLocal(s);
    subscribers.add(sub);
    ensurePolling();
    return () => {
      subscribers.delete(sub);
      if (subscribers.size === 0) stopPolling();
    };
  }, [enabled]);

  return local;
}
