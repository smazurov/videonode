import { useEffect, useState } from 'react';
import type { components } from '../lib/api.generated';
import { api, unwrap } from '../lib/api';
import {
  subscribeConnectionStatus,
  getGlobalConnectionStatus,
  subscribeProcesses,
  subscribeProcessRemoved,
} from './useSSEManager';

export type ProcessEntry = components['schemas']['ProcessEntry'];
type ProcessesEvent = components['schemas']['ProcessesEvent'];
type ProcessRemovedEvent = components['schemas']['ProcessRemovedEvent'];

interface ProcessesState {
  processes: ProcessEntry[];
  loading: boolean;
  error: string | null;
}

// Process stats/state changes arrive over the dedicated SSE stream: the
// `processes` event (immediate on transitions, every 2s while anything runs)
// is the steady-state source of truth, and `process-removed` drops a row the
// moment a process leaves the pool. The REST endpoint is hit only to seed the
// initial paint and to resync after a reconnect — there is no recurring poll.
//
// Shared, refcounted store: every component that calls useProcesses reads the
// same SSE-fed state instead of opening its own stream. A stream-detail page
// mounts several panels that each need process state; one stream fans out to
// all of them.
let shared: ProcessesState = { processes: [], loading: false, error: null };
const subscribers = new Set<(s: ProcessesState) => void>();
// Abort any prior seed/resync request before issuing another so they can never
// stack against a slow/dead backend (the global fetch timeout caps each).
let inFlight: AbortController | null = null;
let connUnsub: (() => void) | null = null;
let processesUnsub: (() => void) | null = null;
let removedUnsub: (() => void) | null = null;
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

// A removed process emits no further events, so drop its row on the explicit
// signal. No-op when the id isn't present (already reconciled by a poll).
function handleProcessRemoved(event: ProcessRemovedEvent): void {
  const id = event.id;
  if (!id) return;
  const next = shared.processes.filter((p) => p.id !== id);
  if (next.length === shared.processes.length) return;
  shared = { ...shared, processes: next };
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

function abortInFlight() {
  inFlight?.abort();
  inFlight = null;
}

// Resync on reconnect: a disconnect can hide adds/removes, and the stream is
// silent while idle, so a one-shot fetch reconciles when the rig comes back.
// No fetch while offline — a dead rig gets no doomed /api/processes requests.
function handleConnectionStatus(status: ReturnType<typeof getGlobalConnectionStatus>): void {
  const nowOnline = status === 'online';
  if (nowOnline && !online) void fetchOnce();
  online = nowOnline;
}

function startSharedStream() {
  if (connUnsub) return;
  processesUnsub = subscribeProcesses(handleProcessesEvent);
  removedUnsub = subscribeProcessRemoved(handleProcessRemoved);
  online = getGlobalConnectionStatus() === 'online';
  connUnsub = subscribeConnectionStatus(handleConnectionStatus);
  // Seed the initial paint; subsequent updates arrive over the stream.
  if (online) void fetchOnce();
}

function stopSharedStream() {
  processesUnsub?.();
  processesUnsub = null;
  removedUnsub?.();
  removedUnsub = null;
  connUnsub?.();
  connUnsub = null;
  abortInFlight();
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
    if (subscribers.size === 1) startSharedStream();
    return () => {
      subscribers.delete(sub);
      if (subscribers.size === 0) stopSharedStream();
    };
  }, [enabled]);

  return local;
}
