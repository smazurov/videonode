interface AttributeFilter {
  key: string;
  operator: 'equals' | 'contains' | 'exists';
  value: string;
}

const ALL_LEVELS = ['error', 'warn', 'info', 'debug'];

const CONNECTION_STATUS_CLASSES: Record<string, string> = {
  connected: 'bg-green-500',
  connecting: 'bg-yellow-500 animate-pulse',
  disconnected: 'bg-red-500',
};

const LEVEL_ACTIVE_CLASSES: Record<string, string> = {
  error: 'bg-red-600 text-white',
  warn: 'bg-yellow-600 text-white',
  info: 'bg-blue-600 text-white',
  debug: 'bg-gray-600 text-white',
};

function getLevelButtonClass(level: string, isSelected: boolean): string {
  if (isSelected) {
    return LEVEL_ACTIVE_CLASSES[level] ?? 'bg-gray-600 text-white';
  }
  return 'bg-gray-700 text-gray-400 hover:bg-gray-600';
}

interface LogFiltersProps {
  readonly connectionStatus: 'connecting' | 'connected' | 'disconnected';
  readonly selectedLevels: string[];
  readonly onToggleLevel: (level: string) => void;
  readonly selectedModules: string[];
  readonly onToggleModule: (module: string) => void;
  readonly availableModules: string[];
  readonly globalFilter: string;
  readonly onGlobalFilterChange: (value: string) => void;
  readonly attributeFilters: AttributeFilter[];
  readonly onAddAttributeFilter: () => void;
  readonly onUpdateAttributeFilter: (index: number, updates: Partial<AttributeFilter>) => void;
  readonly onRemoveAttributeFilter: (index: number) => void;
  readonly availableAttributeKeys: string[];
  readonly autoScroll: boolean;
  readonly onAutoScrollChange: (value: boolean) => void;
  readonly onClearFilters: () => void;
  readonly onClearLogs: () => void;
}

export function LogFilters({
  connectionStatus,
  selectedLevels,
  onToggleLevel,
  selectedModules,
  onToggleModule,
  availableModules,
  globalFilter,
  onGlobalFilterChange,
  attributeFilters,
  onAddAttributeFilter,
  onUpdateAttributeFilter,
  onRemoveAttributeFilter,
  availableAttributeKeys,
  autoScroll,
  onAutoScrollChange,
  onClearFilters,
  onClearLogs,
}: LogFiltersProps) {
  const statusClass = CONNECTION_STATUS_CLASSES[connectionStatus] ?? 'bg-red-500';

  return (
    <>
      {/* Main Filter Bar */}
      <div className="flex flex-wrap items-center gap-2 p-2 bg-gray-800 border-b border-gray-700 shrink-0">
        {/* Connection Status */}
        <div className="flex items-center gap-1.5 pr-3 border-r border-gray-600">
          <div className={`w-2 h-2 rounded-full ${statusClass}`} />
          <span className="text-xs text-gray-400">{connectionStatus}</span>
        </div>

        {/* Level Toggles */}
        <div className="flex gap-1">
          {ALL_LEVELS.map(level => (
            <button
              key={level}
              onClick={() => onToggleLevel(level)}
              className={`px-2 py-0.5 rounded text-xs font-medium transition-colors ${getLevelButtonClass(level, selectedLevels.includes(level))}`}
            >
              {level.toUpperCase()}
            </button>
          ))}
        </div>

        {/* Module Filter */}
        {availableModules.length > 0 && (
          <select
            value=""
            onChange={e => e.target.value && onToggleModule(e.target.value)}
            className="px-2 py-0.5 text-xs bg-gray-700 border border-gray-600 rounded text-gray-300"
          >
            <option value="">+ Module</option>
            {availableModules.filter(m => !selectedModules.includes(m)).map(m => (
              <option key={m} value={m}>{m}</option>
            ))}
          </select>
        )}

        {/* Selected Modules */}
        {selectedModules.map(module => (
          <span
            key={module}
            className="flex items-center gap-1 px-2 py-0.5 text-xs bg-cyan-600 text-white rounded"
          >
            {module}
            <button onClick={() => onToggleModule(module)} className="hover:text-cyan-200">×</button>
          </span>
        ))}

        {/* Search */}
        <input
          type="text"
          placeholder="Search..."
          value={globalFilter}
          onChange={e => onGlobalFilterChange(e.target.value)}
          className="px-2 py-0.5 text-xs bg-gray-700 border border-gray-600 rounded text-gray-300 placeholder-gray-500 w-32"
        />

        {/* Add Attribute Filter */}
        {availableAttributeKeys.length > 0 && (
          <button
            onClick={onAddAttributeFilter}
            className="px-2 py-0.5 text-xs bg-gray-700 text-gray-300 rounded hover:bg-gray-600"
          >
            + Attr
          </button>
        )}

        <div className="flex-1" />

        {/* Auto-scroll */}
        <label className="flex items-center gap-1 text-xs text-gray-400">
          <input
            type="checkbox"
            checked={autoScroll}
            onChange={e => onAutoScrollChange(e.target.checked)}
            className="rounded bg-gray-700 border-gray-600 text-blue-500"
          />
          Follow
        </label>

        {/* Clear buttons */}
        <button
          onClick={onClearFilters}
          className="px-2 py-0.5 text-xs bg-gray-700 text-gray-300 rounded hover:bg-gray-600"
        >
          Reset
        </button>
        <button
          onClick={onClearLogs}
          className="px-2 py-0.5 text-xs bg-gray-700 text-gray-300 rounded hover:bg-gray-600"
        >
          Clear
        </button>
      </div>

      {/* Attribute Filters Row */}
      {attributeFilters.length > 0 && (
        <div className="flex flex-wrap items-center gap-2 px-2 py-1.5 bg-gray-800 border-b border-gray-700 shrink-0">
          {attributeFilters.map((filter, index) => (
            <div key={index} className="flex items-center gap-1 bg-gray-700 rounded px-2 py-0.5">
              <select
                value={filter.key}
                onChange={e => onUpdateAttributeFilter(index, { key: e.target.value })}
                className="bg-transparent text-xs text-purple-400"
              >
                {availableAttributeKeys.map(key => (
                  <option key={key} value={key}>{key}</option>
                ))}
              </select>
              <select
                value={filter.operator}
                onChange={e => onUpdateAttributeFilter(index, { operator: e.target.value as AttributeFilter['operator'] })}
                className="bg-transparent text-xs text-gray-400"
              >
                <option value="contains">~</option>
                <option value="equals">=</option>
                <option value="exists">?</option>
              </select>
              {filter.operator !== 'exists' && (
                <input
                  type="text"
                  value={filter.value}
                  onChange={e => onUpdateAttributeFilter(index, { value: e.target.value })}
                  placeholder="value"
                  className="w-16 bg-transparent text-xs text-yellow-300 placeholder-gray-500"
                />
              )}
              <button
                onClick={() => onRemoveAttributeFilter(index)}
                className="text-gray-500 hover:text-gray-300"
              >
                ×
              </button>
            </div>
          ))}
        </div>
      )}
    </>
  );
}

export type { AttributeFilter };
export { ALL_LEVELS };
