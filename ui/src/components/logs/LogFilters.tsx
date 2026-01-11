import { MultiSelect, type MultiSelectOption } from '../MultiSelect';

interface AttributeFilter {
  key: string;
  operator: 'equals' | 'contains' | 'exists';
  value: string;
}

const ALL_LEVELS = ['error', 'warn', 'info', 'debug'];

const LEVEL_COLORS: Record<string, string> = {
  error: 'text-red-400',
  warn: 'text-yellow-400',
  info: 'text-blue-400',
  debug: 'text-gray-400',
};

const LEVEL_OPTIONS: MultiSelectOption[] = ALL_LEVELS.map(level => ({
  value: level,
  label: level.toUpperCase(),
  color: LEVEL_COLORS[level] ?? 'text-gray-300',
}));

const CONNECTION_STATUS_CLASSES: Record<string, string> = {
  connected: 'bg-green-500',
  connecting: 'bg-yellow-500 animate-pulse',
  disconnected: 'bg-red-500',
};

interface LogFiltersProps {
  readonly connectionStatus: 'connecting' | 'connected' | 'disconnected';
  readonly selectedLevels: string[];
  readonly onSelectedLevelsChange: (levels: string[]) => void;
  readonly selectedModules: string[];
  readonly onSelectedModulesChange: (modules: string[]) => void;
  readonly availableModules: string[];
  readonly inlineAttributes: string[];
  readonly onInlineAttributesChange: (attrs: string[]) => void;
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
  onSelectedLevelsChange,
  selectedModules,
  onSelectedModulesChange,
  availableModules,
  inlineAttributes,
  onInlineAttributesChange,
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

  const moduleOptions: MultiSelectOption[] = availableModules.map(m => ({
    value: m,
    label: m,
  }));

  const inlineAttrOptions: MultiSelectOption[] = availableAttributeKeys.map(k => ({
    value: k,
    label: k,
  }));

  return (
    <>
      {/* Main Filter Bar */}
      <div className="flex flex-wrap items-center gap-2 p-2 bg-gray-800 border-b border-gray-700 shrink-0">
        {/* Connection Status */}
        <div className="flex items-center gap-1.5 pr-3 border-r border-gray-600">
          <div className={`w-2 h-2 rounded-full ${statusClass}`} />
          <span className="text-xs text-gray-400">{connectionStatus}</span>
        </div>

        {/* Level Filter */}
        <MultiSelect
          options={LEVEL_OPTIONS}
          selected={selectedLevels}
          onChange={onSelectedLevelsChange}
          placeholder="Levels"
        />

        {/* Module Filter */}
        {moduleOptions.length > 0 && (
          <MultiSelect
            options={moduleOptions}
            selected={selectedModules}
            onChange={onSelectedModulesChange}
            placeholder="Modules"
          />
        )}

        {/* Inline Attributes */}
        {inlineAttrOptions.length > 0 && (
          <MultiSelect
            options={inlineAttrOptions}
            selected={inlineAttributes}
            onChange={onInlineAttributesChange}
            placeholder="Inline"
          />
        )}

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
