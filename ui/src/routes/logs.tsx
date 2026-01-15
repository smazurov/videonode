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
import { useSSE } from '../hooks/useSSE';
import { LogRow, type LogEntry } from '../components/logs/LogRow';
import {
  LogFilters,
  type AttributeFilter,
  ALL_LEVELS,
} from '../components/logs/LogFilters';
import type { SSEStatus } from '../lib/api_sse';

const MAX_LOGS = 10_000;
const LOG_SETTINGS_KEY = 'logSettings';

interface LogSettings {
  selectedLevels: string[];
  selectedModules: string[];
  attributeFilters: AttributeFilter[];
  inlineAttributes: string[];
  globalFilter: string;
  autoScroll: boolean;
}

const DEFAULT_LOG_SETTINGS: LogSettings = {
  selectedLevels: ALL_LEVELS,
  selectedModules: [],
  attributeFilters: [],
  inlineAttributes: [],
  globalFilter: '',
  autoScroll: true,
};

function loadLogSettings(): LogSettings {
  try {
    const stored = localStorage.getItem(LOG_SETTINGS_KEY);
    if (!stored) return DEFAULT_LOG_SETTINGS;
    const parsed = JSON.parse(stored) as Partial<LogSettings>;
    return { ...DEFAULT_LOG_SETTINGS, ...parsed };
  } catch {
    return DEFAULT_LOG_SETTINGS;
  }
}

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

// Map SSE status to simpler connection status for UI
function mapConnectionStatus(status: SSEStatus): 'connecting' | 'connected' | 'disconnected' {
  switch (status) {
    case 'connected': return 'connected';
    case 'connecting': return 'connecting';
    default: return 'disconnected';
  }
}

export default function Logs() {
  const { logout } = useAuthStore();
  const scrollRef = useRef<HTMLDivElement>(null);
  const newestTimestampRef = useRef<string>('');

  // Core state
  const [logs, setLogs] = useState<LogEntry[]>([]);

  // Buffering refs for batching log updates
  const bufferRef = useRef<LogEntry[]>([]);
  const flushTimeoutRef = useRef<number | null>(null);
  const idCounterRef = useRef(0);

  // Filter state (initialized from localStorage)
  const [settings] = useState(loadLogSettings);
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([]);
  const [globalFilter, setGlobalFilter] = useState(settings.globalFilter);
  const [selectedLevels, setSelectedLevels] = useState<string[]>(settings.selectedLevels);
  const [selectedModules, setSelectedModules] = useState<string[]>(settings.selectedModules);
  const [attributeFilters, setAttributeFilters] = useState<AttributeFilter[]>(settings.attributeFilters);
  const [inlineAttributes, setInlineAttributes] = useState<string[]>(settings.inlineAttributes);
  const [autoScroll, setAutoScroll] = useState(settings.autoScroll);

  // Flush buffer to state
  const flushBuffer = useCallback(() => {
    const buffer = bufferRef.current;
    // Filter out duplicates: keep logs newer than last seen
    const toFlush = buffer.filter(log =>
      !newestTimestampRef.current || log.timestamp > newestTimestampRef.current
    );
    bufferRef.current = [];
    flushTimeoutRef.current = null;

    if (toFlush.length > 0) {
      const timestamps = toFlush.map(log => log.timestamp).filter(t => t);
      if (timestamps.length > 0) {
        newestTimestampRef.current = timestamps.reduce((a, b) => a > b ? a : b, '');
      }
      setLogs(prev => [...prev, ...toFlush].slice(-MAX_LOGS));
    }
  }, []);

  const scheduleFlush = useCallback(() => {
    if (!flushTimeoutRef.current) {
      flushTimeoutRef.current = window.setTimeout(flushBuffer, 50);
    }
  }, [flushBuffer]);

  // SSE connection using the abstracted hook
  const { status } = useSSE({
    endpoint: '/api/logs/stream',
    onConnect: () => {
      // Inject synthetic log entry to mark connection
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

  const connectionStatus = mapConnectionStatus(status);

  // Clean up flush timeout on unmount
  useEffect(() => {
    return () => {
      if (flushTimeoutRef.current) {
        window.clearTimeout(flushTimeoutRef.current);
      }
    };
  }, []);

  // Persist filter settings to localStorage
  useEffect(() => {
    const toSave: LogSettings = {
      selectedLevels,
      selectedModules,
      attributeFilters,
      inlineAttributes,
      globalFilter,
      autoScroll,
    };
    try {
      localStorage.setItem(LOG_SETTINGS_KEY, JSON.stringify(toSave));
    } catch {
      // Ignore storage errors
    }
  }, [selectedLevels, selectedModules, attributeFilters, inlineAttributes, globalFilter, autoScroll]);

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

  // eslint-disable-next-line react-hooks/incompatible-library -- React Compiler auto-skips this component
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

  const hasActiveFilters = selectedLevels.length !== ALL_LEVELS.length
    || selectedModules.length > 0
    || attributeFilters.length > 0
    || inlineAttributes.length > 0
    || globalFilter !== '';

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
      <div className="px-2 py-0.5 text-xs text-gray-500 bg-gray-900 border-b border-gray-800 shrink-0 flex items-center gap-1">
        {rows.length.toLocaleString()} / {logs.length.toLocaleString()} logs
        {hasActiveFilters && (
          <button
            onClick={clearFilters}
            className="text-gray-500 hover:text-gray-300 cursor-pointer"
            title="Reset all filters"
          >
            [x]
          </button>
        )}
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
