import { useCallback, useEffect, useRef, useState } from 'react';
import { useSSE } from './useSSE';
import type { LogEntry } from '../components/logs/LogRow';
import type { SSEStatus } from '../lib/api';

interface LogEventData {
  timestamp: string;
  level: string;
  module: string;
  message: string;
  attributes?: Record<string, unknown>;
}

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

function mapConnectionStatus(status: SSEStatus): 'connecting' | 'connected' | 'disconnected' {
  switch (status) {
    case 'connected': return 'connected';
    case 'connecting': return 'connecting';
    default: return 'disconnected';
  }
}

interface UseLogStreamOptions {
  enabled?: boolean;
  maxLogs?: number;
}

interface UseLogStreamResult {
  logs: LogEntry[];
  connectionStatus: 'connecting' | 'connected' | 'disconnected';
  clearLogs: () => void;
}

export function useLogStream({ enabled = true, maxLogs = 10_000 }: UseLogStreamOptions = {}): UseLogStreamResult {
  const [logs, setLogs] = useState<LogEntry[]>([]);

  const bufferRef = useRef<LogEntry[]>([]);
  const flushTimeoutRef = useRef<number | null>(null);
  const idCounterRef = useRef(0);
  const maxLogsRef = useRef(maxLogs);

  useEffect(() => {
    maxLogsRef.current = maxLogs;
  }, [maxLogs]);

  const flushBuffer = useCallback(() => {
    const batch = bufferRef.current;
    bufferRef.current = [];
    flushTimeoutRef.current = null;

    if (batch.length === 0) return;

    setLogs(prev => {
      let result = [...prev];

      for (const entry of batch) {
        const suppressed = entry.attributes['suppressed'];
        if (typeof suppressed === 'number' && suppressed > 0) {
          // Dedup update — find last entry with same message and update it
          let found = false;
          for (let i = result.length - 1; i >= 0; i--) {
            const existing = result[i]!;
            if (existing.message === entry.message) {
              result[i] = { ...existing, attributes: { ...existing.attributes, suppressed } };
              found = true;
              break;
            }
          }
          if (found) continue;
        }
        result.push(entry);
      }

      return result.slice(-maxLogsRef.current);
    });
  }, []);

  const scheduleFlush = useCallback(() => {
    if (!flushTimeoutRef.current) {
      flushTimeoutRef.current = window.setTimeout(flushBuffer, 50);
    }
  }, [flushBuffer]);

  const { status } = useSSE({
    endpoint: '/api/logs/stream',
    enabled,
    onConnect: () => {
      bufferRef.current.push({
        id: String(++idCounterRef.current),
        timestamp: new Date().toISOString(),
        level: 'INFO',
        module: 'system',
        message: 'Log stream connected',
        attributes: {},
      });
      scheduleFlush();
    },
    onMessage: (event) => {
      try {
        const data: unknown = JSON.parse(String(event.data));
        if (!isLogEventData(data)) {
          console.error('Invalid log data format:', event.data);
          return;
        }
        bufferRef.current.push({
          id: String(++idCounterRef.current),
          timestamp: data.timestamp,
          level: data.level,
          module: data.module,
          message: data.message,
          attributes: data.attributes ?? {},
        });
        scheduleFlush();
      } catch (error) {
        console.error('Log parse error:', error, event.data);
      }
    },
  });

  // Clean up flush timeout on unmount
  useEffect(() => {
    return () => {
      if (flushTimeoutRef.current) {
        window.clearTimeout(flushTimeoutRef.current);
      }
    };
  }, []);

  const clearLogs = useCallback(() => setLogs([]), []);

  return {
    logs,
    connectionStatus: mapConnectionStatus(status),
    clearLogs,
  };
}
