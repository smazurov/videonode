import { useCallback, useEffect, useRef, useState } from 'react';
import type { LogEntry } from '../components/logs/LogRow';
import type { SSEStatus } from '../lib/api';
import { subscribeLogStream, mergeLogBatch } from '../lib/logStream';

function mapConnectionStatus(status: SSEStatus): 'connecting' | 'connected' | 'disconnected' {
  switch (status) {
    case 'connected': return 'connected';
    case 'connecting': return 'connecting';
    default: return 'disconnected';
  }
}

export interface LogFilter {
  key: 'source_id' | 'composer_id' | 'stream_id';
  id: string;
}

interface UseLogStreamOptions {
  enabled?: boolean;
  maxLogs?: number;
  filter?: LogFilter;
}

interface UseLogStreamResult {
  logs: LogEntry[];
  connectionStatus: 'connecting' | 'connected' | 'disconnected';
  clearLogs: () => void;
}

export function useLogStream({
  enabled = true,
  maxLogs = 10_000,
  filter,
}: UseLogStreamOptions = {}): UseLogStreamResult {
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [status, setStatus] = useState<SSEStatus>('disconnected');

  const maxLogsRef = useRef(maxLogs);
  const filterRef = useRef(filter);
  useEffect(() => {
    maxLogsRef.current = maxLogs;
    filterRef.current = filter;
  });

  const filterKey = filter?.key;
  const filterId = filter?.id;

  useEffect(() => {
    if (!enabled) return;

    return subscribeLogStream({
      onBatch: (batch, isBackfill) => {
        const f = filterRef.current;
        const relevant = f ? batch.filter((e) => e.attributes[f.key] === f.id) : batch;
        // Backfill replaces (resets on filter change); live batches append.
        if (isBackfill) {
          setLogs(mergeLogBatch([], relevant, maxLogsRef.current));
          return;
        }
        if (relevant.length === 0) return;
        setLogs((prev) => mergeLogBatch(prev, relevant, maxLogsRef.current));
      },
      onStatus: setStatus,
    });
  }, [enabled, filterKey, filterId]);

  const clearLogs = useCallback(() => setLogs([]), []);

  return {
    logs,
    connectionStatus: mapConnectionStatus(status),
    clearLogs,
  };
}
