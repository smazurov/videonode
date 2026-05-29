import { SSEClient, type SSEStatus } from './api';
import type { LogEntry } from '../components/logs/LogRow';

export interface LogStreamSubscriber {
  // `isBackfill` is true only for the one-time snapshot delivered at subscribe
  // time, signalling consumers to replace rather than append.
  onBatch: (entries: LogEntry[], isBackfill: boolean) => void;
  onStatus: (status: SSEStatus) => void;
}

interface LogEventData {
  timestamp: string;
  level: string;
  module: string;
  message: string;
  attributes?: Record<string, unknown>;
}

const GLOBAL_MAX = 10_000;
const FLUSH_DELAY = 50;

// Module-level singleton: one EventSource to /api/logs/stream shared by every
// subscriber (the full logs route plus any per-entity panels). Mirrors the
// global-connection refcounting in useSSEManager so we never open more than one
// firehose no matter how many views mount.
let client: SSEClient<'/api/logs/stream'> | null = null;
let status: SSEStatus = 'disconnected';
let globalLogs: LogEntry[] = [];
let idCounter = 0;
const subscribers = new Set<LogStreamSubscriber>();

let pending: LogEntry[] = [];
let flushTimer: number | null = null;

function isLogEventData(data: unknown): data is LogEventData {
  return (
    typeof data === 'object' &&
    data !== null &&
    'timestamp' in data &&
    'level' in data &&
    'module' in data &&
    'message' in data
  );
}

// Append entry to list, folding dedup "suppressed" updates into the last
// matching message rather than adding a new row.
function mergeInto(list: LogEntry[], entry: LogEntry): void {
  const suppressed = entry.attributes['suppressed'];
  if (typeof suppressed === 'number' && suppressed > 0) {
    for (let i = list.length - 1; i >= 0; i--) {
      const existing = list[i]!;
      if (existing.message === entry.message) {
        list[i] = { ...existing, attributes: { ...existing.attributes, suppressed } };
        return;
      }
    }
  }
  list.push(entry);
}

/**
 * Merge a batch of entries into a prior list, applying dedup folding and
 * capping to `max`. Shared by the singleton (global buffer) and each
 * useLogStream consumer (its filtered, capped view) so dedup stays consistent.
 */
export function mergeLogBatch(prev: LogEntry[], batch: LogEntry[], max: number): LogEntry[] {
  const result = [...prev];
  for (const entry of batch) {
    mergeInto(result, entry);
  }
  return result.length > max ? result.slice(-max) : result;
}

function flush(): void {
  flushTimer = null;
  const batch = pending;
  pending = [];
  if (batch.length === 0) return;

  globalLogs = mergeLogBatch(globalLogs, batch, GLOBAL_MAX);
  for (const sub of subscribers) {
    sub.onBatch(batch, false);
  }
}

function scheduleFlush(): void {
  if (flushTimer === null) {
    flushTimer = window.setTimeout(flush, FLUSH_DELAY);
  }
}

function pushEntry(entry: Omit<LogEntry, 'id'>): void {
  pending.push({ id: String(++idCounter), ...entry });
  scheduleFlush();
}

function ensureClient(): void {
  if (client) return;

  client = new SSEClient({
    endpoint: '/api/logs/stream',
    onStatusChange: (s) => {
      status = s;
      for (const sub of subscribers) {
        sub.onStatus(s);
      }
    },
    onConnect: () => {
      pushEntry({
        timestamp: new Date().toISOString(),
        level: 'INFO',
        module: 'system',
        message: 'Log stream connected',
        attributes: {},
      });
    },
  });

  client.onMessage((event) => {
    try {
      const data: unknown = JSON.parse(String(event.data));
      if (!isLogEventData(data)) {
        console.error('Invalid log data format:', event.data);
        return;
      }
      pushEntry({
        timestamp: data.timestamp,
        level: data.level,
        module: data.module,
        message: data.message,
        attributes: data.attributes ?? {},
      });
    } catch (error) {
      console.error('Log parse error:', error, event.data);
    }
  });

  client.connect();
}

function teardown(): void {
  client?.disconnect();
  client = null;
  status = 'disconnected';
  globalLogs = [];
  pending = [];
  if (flushTimer !== null) {
    window.clearTimeout(flushTimer);
    flushTimer = null;
  }
}

/**
 * Subscribe to the shared log stream. Lazily opens the single EventSource on
 * the first subscriber and tears it down when the last one leaves. The new
 * subscriber is immediately backfilled with the current buffer and status.
 */
export function subscribeLogStream(sub: LogStreamSubscriber): () => void {
  subscribers.add(sub);
  ensureClient();
  sub.onBatch(globalLogs.slice(), true);
  sub.onStatus(status);

  return () => {
    subscribers.delete(sub);
    if (subscribers.size === 0) {
      teardown();
    }
  };
}
