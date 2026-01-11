"use no memo";

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  useReactTable,
  getCoreRowModel,
  getFilteredRowModel,
  ColumnFiltersState,
  FilterFn,
  createColumnHelper,
} from '@tanstack/react-table';
import { useAuthStore } from '../hooks/useAuthStore';
import { Header } from '../components/Header';
import { API_BASE_URL } from '../lib/api';
import { LogRow, type LogEntry } from '../components/logs/LogRow';
import {
  LogFilters,
  type AttributeFilter,
  ALL_LEVELS,
} from '../components/logs/LogFilters';

const MAX_LOGS = 10_000;

interface LogEventData {
  seq?: number;
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

// Filter functions
const multiLevelFilter: FilterFn<LogEntry> = (row, _columnId, filterValue: string[]) => {
  if (!filterValue || filterValue.length === 0 || filterValue.length === ALL_LEVELS.length) {
    return true;
  }
  return filterValue.includes(row.original.level);
};

const moduleFilter: FilterFn<LogEntry> = (row, _columnId, filterValue: string[]) => {
  if (!filterValue || filterValue.length === 0) {
    return true;
  }
  return filterValue.includes(row.original.module);
};

const columnHelper = createColumnHelper<LogEntry>();

export default function Logs() {
  const { logout } = useAuthStore();
  const scrollRef = useRef<HTMLDivElement>(null);
  const lastSeenSeqRef = useRef<number>(0);

  // Core state
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [connectionStatus, setConnectionStatus] = useState<'connecting' | 'connected' | 'disconnected'>('connecting');
  const [autoScroll, setAutoScroll] = useState(true);

  // Filter state
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([]);
  const [globalFilter, setGlobalFilter] = useState('');
  const [selectedLevels, setSelectedLevels] = useState<string[]>(ALL_LEVELS);
  const [selectedModules, setSelectedModules] = useState<string[]>([]);
  const [attributeFilters, setAttributeFilters] = useState<AttributeFilter[]>([]);
  const [inlineAttributes, setInlineAttributes] = useState<string[]>([]);

  // Derived data
  const availableModules = useMemo(() =>
    [...new Set(logs.map(l => l.module))].sort((a, b) => a.localeCompare(b)),
    [logs]
  );

  const availableAttributeKeys = useMemo(() => {
    const keys = new Set<string>();
    for (const log of logs) {
      for (const key of Object.keys(log.attributes)) {
        keys.add(key);
      }
    }
    return [...keys].sort((a, b) => a.localeCompare(b));
  }, [logs]);

  // Apply attribute filters before table
  const filteredByAttributes = useMemo(() => {
    if (attributeFilters.length === 0) return logs;
    return logs.filter(log => {
      for (const filter of attributeFilters) {
        const value = log.attributes[filter.key];
        const strValue = String(value ?? '').toLowerCase();
        const filterVal = filter.value.toLowerCase();
        switch (filter.operator) {
          case 'equals':
            if (strValue !== filterVal) return false;
            break;
          case 'contains':
            if (!strValue.includes(filterVal)) return false;
            break;
          case 'exists':
            if (!(filter.key in log.attributes)) return false;
            break;
        }
      }
      return true;
    });
  }, [logs, attributeFilters]);

  // Table columns
  const columns = useMemo(() => [
    columnHelper.accessor('level', { filterFn: multiLevelFilter }),
    columnHelper.accessor('module', { filterFn: moduleFilter }),
    columnHelper.accessor('message', {}),
  ], []);

  // Global filter that searches message, module, and attributes
  const globalFilterFn: FilterFn<LogEntry> = useCallback((row, _columnId, filterValue: string) => {
    if (!filterValue) return true;
    const search = filterValue.toLowerCase();
    const log = row.original;
    if (log.message.toLowerCase().includes(search)) return true;
    if (log.module.toLowerCase().includes(search)) return true;
    for (const [key, value] of Object.entries(log.attributes)) {
      if (key.toLowerCase().includes(search)) return true;
      if (String(value).toLowerCase().includes(search)) return true;
    }
    return false;
  }, []);

  // Table instance - incompatible-library warning suppressed: "use no memo" directive handles this
  // eslint-disable-next-line react-hooks/incompatible-library
  const table = useReactTable({
    data: filteredByAttributes,
    columns,
    state: { columnFilters, globalFilter },
    onColumnFiltersChange: setColumnFilters,
    onGlobalFilterChange: setGlobalFilter,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    globalFilterFn,
  });

  const { rows } = table.getRowModel();

  // Sync level/module filters to table
  useEffect(() => {
    setColumnFilters([
      { id: 'level', value: selectedLevels },
      { id: 'module', value: selectedModules },
    ]);
  }, [selectedLevels, selectedModules]);

  // Auto-scroll to bottom when new logs arrive
  useEffect(() => {
    if (autoScroll && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [rows.length, autoScroll]);

  // SSE connection
  useEffect(() => {
    const credentials = localStorage.getItem('auth_credentials') || '';
    const sseUrl = `${API_BASE_URL}/api/logs/stream?auth=${encodeURIComponent(credentials)}`;

    let eventSource: EventSource | null = null;
    let buffer: LogEntry[] = [];
    let flushTimeout: number | null = null;
    let reconnectTimeout: number | null = null;
    let reconnectDelay = 5000;

    const flushBuffer = () => {
      // Filter out duplicates: keep logs with seq=0 (synthetic/legacy) or seq > lastSeen
      const toFlush = buffer.filter(log => log.seq === 0 || log.seq > lastSeenSeqRef.current);
      buffer = [];
      flushTimeout = null;
      if (toFlush.length > 0) {
        const seqs = toFlush.map(log => log.seq).filter(s => s > 0);
        if (seqs.length > 0) {
          lastSeenSeqRef.current = Math.max(...seqs);
        }
        setLogs(prev => [...prev, ...toFlush].slice(-MAX_LOGS));
      }
    };

    const scheduleReconnect = () => {
      reconnectTimeout = window.setTimeout(() => {
        connect();
        reconnectDelay = Math.min(reconnectDelay * 2, 60000);
      }, reconnectDelay);
    };

    const connect = () => {
      setConnectionStatus('connecting');
      eventSource = new EventSource(sseUrl);
      eventSource.onopen = () => {
        setConnectionStatus('connected');
        reconnectDelay = 5000;
        // Inject synthetic log entry to mark connection/reconnection
        buffer.push({
          id: `connect-${Date.now()}`,
          seq: 0,
          timestamp: new Date().toISOString(),
          level: 'INFO',
          module: 'system',
          message: 'Log stream connected',
          attributes: {},
        });
        if (!flushTimeout) {
          flushTimeout = window.setTimeout(flushBuffer, 50);
        }
      };

      eventSource.onmessage = (event: MessageEvent) => {
        try {
          const data: unknown = JSON.parse(String(event.data));
          if (!isLogEventData(data)) {
            console.error('Invalid log data format:', event.data);
            return;
          }
          const seq = data.seq ?? 0;
          buffer.push({
            id: String(seq),
            seq,
            timestamp: data.timestamp,
            level: data.level,
            module: data.module,
            message: data.message,
            attributes: data.attributes ?? {},
          });

          if (!flushTimeout) {
            flushTimeout = window.setTimeout(flushBuffer, 50);
          }
        } catch (error) {
          console.error('Log parse error:', error, event.data);
        }
      };

      eventSource.onerror = () => {
        setConnectionStatus('disconnected');
        eventSource?.close();
        eventSource = null;
        scheduleReconnect();
      };
    };

    connect();

    return () => {
      if (flushTimeout) window.clearTimeout(flushTimeout);
      if (reconnectTimeout) window.clearTimeout(reconnectTimeout);
      eventSource?.close();
    };
  }, []);

  // Handlers
  const addAttributeFilter = () => {
    const firstKey = availableAttributeKeys[0];
    if (firstKey) {
      setAttributeFilters(prev => [...prev, { key: firstKey, operator: 'contains' as const, value: '' }]);
    }
  };

  const updateAttributeFilter = (index: number, updates: Partial<AttributeFilter>) => {
    setAttributeFilters(prev => prev.map((f, i) => i === index ? { ...f, ...updates } : f));
  };

  const removeAttributeFilter = (index: number) => {
    setAttributeFilters(prev => prev.filter((_, i) => i !== index));
  };

  const clearFilters = () => {
    setSelectedLevels(ALL_LEVELS);
    setSelectedModules([]);
    setGlobalFilter('');
    setAttributeFilters([]);
    setInlineAttributes([]);
  };

  const handleLogout = useCallback(() => logout(), [logout]);

  return (
    <div className="h-screen flex flex-col bg-gray-900">
      <Header onLogout={handleLogout} />

      <div className="mt-8 shrink-0">
        <LogFilters
          connectionStatus={connectionStatus}
          selectedLevels={selectedLevels}
          onSelectedLevelsChange={setSelectedLevels}
          selectedModules={selectedModules}
          onSelectedModulesChange={setSelectedModules}
          availableModules={availableModules}
          inlineAttributes={inlineAttributes}
          onInlineAttributesChange={setInlineAttributes}
          globalFilter={globalFilter}
          onGlobalFilterChange={setGlobalFilter}
          attributeFilters={attributeFilters}
          onAddAttributeFilter={addAttributeFilter}
          onUpdateAttributeFilter={updateAttributeFilter}
          onRemoveAttributeFilter={removeAttributeFilter}
          availableAttributeKeys={availableAttributeKeys}
          autoScroll={autoScroll}
          onAutoScrollChange={setAutoScroll}
          onClearFilters={clearFilters}
          onClearLogs={() => setLogs([])}
        />
      </div>

      {/* Log count */}
      <div className="px-2 py-0.5 text-xs text-gray-500 bg-gray-900 border-b border-gray-800 shrink-0">
        {rows.length.toLocaleString()} / {logs.length.toLocaleString()} logs
      </div>

      {/* Log viewer - CSS content-visibility for native virtualization */}
      <div ref={scrollRef} className="flex-1 overflow-auto bg-gray-900 font-mono text-sm min-h-0">
        {rows.map(row => (
          <LogRow key={row.original.id} log={row.original} inlineAttributes={inlineAttributes} />
        ))}
      </div>
    </div>
  );
}
