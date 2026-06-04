import { useEffect, useState } from 'react';
import type { components } from '../lib/api.generated';
import { api, unwrap } from '../lib/api';
import {
  subscribeConnectionStatus,
  getGlobalConnectionStatus,
  subscribeProcesses,
} from './useSSEManager';

export type ProcessEntry = components['schemas']['ProcessEntry'];
type ProcessesEvent = components['schemas']['ProcessesEvent'];

interface ProcessesState {
  processes: ProcessEntry[];
  loading: boolean;
  error: string | null;
}

// Process stats/state changes arrive over the dedicated SSE stream (immediate
// on transitions, every 2s while anything runs). The poll is now only a
// low-rate reconciliation backstop: it seeds the initial paint and drops rows
// for processes that were deleted (a removed process emits no further events).
const POLL_INTERVAL_MS = 30_000;

// Shared, refcounted store: every component that calls useProcesses reads the
// same SSE-fed state instead of starting its own timer/subscription. A
// stream-detail page mounts several panels that each need process state; one
// stream fans out to all of them.
let shared: ProcessesState = { processes: [], loading: false, error: null };
const subscribers = new Set<(s: ProcessesState) => void>();
let timer: number | null = null;
// Abort the prior request before each tick so polls can never stack against a
// slow/dead backend (the global fetch timeout caps each at ~8s regardless).
let inFlight: AbortController | null = null;
let connUnsub: (() => void) | null = null;
let processesUnsub: (() => void) | null = null;
let online = true;

function emit() {
  for (const fn of subscribers) fn(shared);
}

// SSE push is the steady-state source of truth: each event carries the full
// supervised set, so replace wholesale. ProcessInfo is a structural subset of
// ProcessEntry (no source-registry join fields), so the rows drop straight in.
function handleProcessesEvent(event: ProcessesEvent): void {
  shared = { processes: event.processes ?? [], loading: false, error: null };
  emit();
}

async function fetchOnce() {
  inFlight?.abort();
  const controller = new AbortController();
  inFlight = controller;
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
  } finally {
    if (inFlight === controller) inFlight = null;
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
  inFlight?.abort();
  inFlight = null;
}

// Poll only while online: a dead rig gets no doomed /api/processes requests.
// The shared subscription resumes polling the moment status returns to online.
function handleConnectionStatus(status: ReturnType<typeof getGlobalConnectionStatus>): void {
  online = status === 'online';
  if (online) ensurePolling();
  else stopPolling();
}

function startSharedPolling() {
  if (connUnsub) return;
  processesUnsub = subscribeProcesses(handleProcessesEvent);
  online = getGlobalConnectionStatus() === 'online';
  connUnsub = subscribeConnectionStatus(handleConnectionStatus);
  if (online) ensurePolling();
}

function stopSharedPolling() {
  processesUnsub?.();
  processesUnsub = null;
  connUnsub?.();
  connUnsub = null;
  stopPolling();
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
    if (subscribers.size === 1) startSharedPolling();
    return () => {
      subscribers.delete(sub);
      if (subscribers.size === 0) stopSharedPolling();
    };
  }, [enabled]);

  return local;
}
