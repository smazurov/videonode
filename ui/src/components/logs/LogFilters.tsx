import { MultiSelect, type MultiSelectOption } from '../MultiSelect';
import { Checkbox } from '../Checkbox';
import { logLevelClasses, connectionStatusClasses } from '../../design/status';

interface AttributeFilter {
  key: string;
  operator: 'equals' | 'contains' | 'exists';
  value: string;
}

const ALL_LEVELS = ['error', 'warn', 'info', 'debug'];

const LEVEL_OPTIONS: MultiSelectOption[] = ALL_LEVELS.map(level => ({
  value: level,
  label: level.toUpperCase(),
  color: logLevelClasses(level),
}));

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
  const statusClass = connectionStatusClasses(connectionStatus);

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
      <div className="flex flex-wrap items-center gap-2 p-2 bg-surface-muted border-b border-border shrink-0">
        {/* Connection Status */}
        <div className="flex items-center gap-1.5 pr-3 border-r border-border">
          <div className={`w-2 h-2 rounded-full ${statusClass}`} />
          <span className="text-xs text-fg-muted">{connectionStatus}</span>
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
          aria-label="Search logs"
          value={globalFilter}
          onChange={e => onGlobalFilterChange(e.target.value)}
          className="px-2 py-0.5 text-xs bg-surface border border-border rounded text-fg placeholder:text-fg-subtle w-32 focus:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring"
        />

        {/* Add Attribute Filter */}
        {availableAttributeKeys.length > 0 && (
          <button
            onClick={onAddAttributeFilter}
            className="px-2 py-0.5 text-xs bg-surface text-fg-muted rounded hover:bg-surface-muted focus-visible:ring-2 focus-visible:ring-focus-ring"
          >
            + Attr
          </button>
        )}

        <div className="flex-1" />

        {/* Auto-scroll */}
        <Checkbox
          checked={autoScroll}
          onChange={e => onAutoScrollChange(e.target.checked)}
          label={<span className="text-xs text-fg-muted">Follow</span>}
        />

        {/* Clear buttons */}
        <button
          onClick={onClearFilters}
          className="px-2 py-0.5 text-xs bg-surface text-fg-muted rounded hover:bg-surface-muted focus-visible:ring-2 focus-visible:ring-focus-ring"
        >
          Reset
        </button>
        <button
          onClick={onClearLogs}
          className="px-2 py-0.5 text-xs bg-surface text-fg-muted rounded hover:bg-surface-muted focus-visible:ring-2 focus-visible:ring-focus-ring"
        >
          Clear
        </button>
      </div>

      {/* Attribute Filters Row */}
      {attributeFilters.length > 0 && (
        <div className="flex flex-wrap items-center gap-2 px-2 py-1.5 bg-surface-muted border-b border-border shrink-0">
          {attributeFilters.map((filter, index) => {
            const filterKey = `${filter.key}:${filter.operator}:${filter.value}`;
            return (
              <div key={filterKey} className="flex items-center gap-1.5">
                <select
                  value={filter.key}
                  aria-label={`Attribute key ${index + 1}`}
                  onChange={e => onUpdateAttributeFilter(index, { key: e.target.value })}
                  className="pl-2 pr-7 py-0.5 text-xs bg-surface border border-border rounded text-canvas-soft-fg cursor-pointer bg-right focus:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring"
                >
                  {availableAttributeKeys.map(key => (
                    <option key={key} value={key}>{key}</option>
                  ))}
                </select>
                <select
                  value={filter.operator}
                  aria-label={`Attribute operator ${index + 1}`}
                  onChange={e => onUpdateAttributeFilter(index, { operator: e.target.value as AttributeFilter['operator'] })}
                  className="pl-2 pr-7 py-0.5 text-xs bg-surface border border-border rounded text-fg-muted cursor-pointer bg-right focus:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring"
                >
                  <option value="contains">~</option>
                  <option value="equals">=</option>
                  <option value="exists">?</option>
                </select>
                {filter.operator !== 'exists' && (
                  <input
                    type="text"
                    value={filter.value}
                    aria-label={`Attribute value ${index + 1}`}
                    onChange={e => onUpdateAttributeFilter(index, { value: e.target.value })}
                    placeholder="value"
                    className="w-20 px-2 py-0.5 text-xs bg-surface border border-border rounded text-srt-soft-fg placeholder:text-fg-subtle focus:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring"
                  />
                )}
                <button
                  onClick={() => onRemoveAttributeFilter(index)}
                  aria-label={`Remove attribute filter ${index + 1}`}
                  className="px-1.5 py-0.5 text-xs text-fg-subtle hover:text-fg bg-surface border border-border rounded hover:bg-surface-muted focus-visible:ring-2 focus-visible:ring-focus-ring"
                >
                  ×
                </button>
              </div>
            );
          })}
        </div>
      )}
    </>
  );
}

export type { AttributeFilter };
export { ALL_LEVELS };
