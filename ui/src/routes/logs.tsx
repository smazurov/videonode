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
import { useLogStream } from '../hooks/useLogStream';
import { LogRow, type LogEntry } from '../components/logs/LogRow';
import {
  LogFilters,
  type AttributeFilter,
  ALL_LEVELS,
} from '../components/logs/LogFilters';
import { ProcessList } from '../components/processes/ProcessList';

const LOG_SETTINGS_KEY = 'logSettings';
const PROCESSES_VISIBLE_KEY = 'logsProcessesVisible';

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
  const logout = useAuthStore((s) => s.logout);
  const scrollRef = useRef<HTMLDivElement>(null);

  const { logs, connectionStatus, clearLogs } = useLogStream({ enabled: true });

  // Filter state (initialized from localStorage)
  const [settings] = useState(loadLogSettings);
  // Derived from selectedLevels/selectedModules — no sync effect needed.
  // (The table never mutates column filters independently in this setup.)

  const [globalFilter, setGlobalFilter] = useState(settings.globalFilter);
  const [selectedLevels, setSelectedLevels] = useState<string[]>(settings.selectedLevels);
  const [selectedModules, setSelectedModules] = useState<string[]>(settings.selectedModules);
  const [attributeFilters, setAttributeFilters] = useState<AttributeFilter[]>(settings.attributeFilters);
  const [inlineAttributes, setInlineAttributes] = useState<string[]>(settings.inlineAttributes);
  const [autoScroll, setAutoScroll] = useState(settings.autoScroll);
  const [processesVisible, setProcessesVisible] = useState(() => {
    try {
      const stored = localStorage.getItem(PROCESSES_VISIBLE_KEY);
      return stored === null ? true : stored === 'true';
    } catch {
      return true;
    }
  });

  useEffect(() => {
    try {
      localStorage.setItem(PROCESSES_VISIBLE_KEY, String(processesVisible));
    } catch {
      // Ignore storage errors
    }
  }, [processesVisible]);

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

  const columnFilters: ColumnFiltersState = useMemo(() => [
    { id: 'level', value: selectedLevels },
    { id: 'module', value: selectedModules },
  ], [selectedLevels, selectedModules]);

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
          onClearLogs={clearLogs}
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
        <button
          onClick={() => setProcessesVisible(v => !v)}
          className="ml-auto text-gray-500 hover:text-gray-300 cursor-pointer"
          title={processesVisible ? "Hide process list" : "Show process list"}
        >
          {processesVisible ? '› processes' : '‹ processes'}
        </button>
      </div>

      {/* Log viewer + process list side-by-side */}
      <div className="flex-1 flex min-h-0">
        <div ref={scrollRef} className="flex-1 overflow-auto bg-gray-900 font-mono text-sm min-h-0">
          {rows.map(row => (
            <LogRow key={row.original.id} log={row.original} inlineAttributes={inlineAttributes} />
          ))}
        </div>
        {processesVisible && (
          <div className="w-96 shrink-0 min-h-0">
            <ProcessList />
          </div>
        )}
      </div>
    </div>
  );
}
