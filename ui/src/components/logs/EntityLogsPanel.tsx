import { useEffect, useMemo, useRef, useState } from 'react';
import { useLogStream, type LogFilter } from '../../hooks/useLogStream';
import { Card } from '../Card';
import { Checkbox } from '../Checkbox';
import { SectionHeader } from '../primitives/SectionHeader';
import { LogRow } from './LogRow';
import { ALL_LEVELS } from './LogFilters';
import { MultiSelect, type MultiSelectOption } from '../MultiSelect';
import { logLevelClasses, connectionStatusClasses } from '../../design/status';

export interface EntityLogsPanelProps {
  filter: LogFilter;
  title?: string;
  description?: string;
}

const LEVEL_OPTIONS: MultiSelectOption[] = ALL_LEVELS.map((level) => ({
  value: level,
  label: level.toUpperCase(),
  color: logLevelClasses(level),
}));

export function EntityLogsPanel({
  filter,
  title = 'Logs',
  description,
}: Readonly<EntityLogsPanelProps>) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const [autoScroll, setAutoScroll] = useState(true);
  const [selectedLevels, setSelectedLevels] = useState<string[]>(ALL_LEVELS);

  const { logs, connectionStatus, clearLogs } = useLogStream({ filter, maxLogs: 2_000 });

  const filteredLogs = useMemo(
    () =>
      selectedLevels.length < ALL_LEVELS.length
        ? logs.filter((log) => selectedLevels.includes(log.level))
        : logs,
    [logs, selectedLevels],
  );

  useEffect(() => {
    if (autoScroll && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [filteredLogs.length, autoScroll]);

  const statusColor = connectionStatusClasses(connectionStatus);

  return (
    <Card padding="lg">
      <div className="flex items-center justify-between gap-3">
        <SectionHeader title={title} description={description} />
        <div className="flex items-center gap-3 shrink-0">
          <div className="flex items-center gap-1.5">
            <div className={`w-2 h-2 rounded-full ${statusColor}`} />
            <span className="text-xs text-fg-muted">{connectionStatus}</span>
          </div>
          <MultiSelect
            options={LEVEL_OPTIONS}
            selected={selectedLevels}
            onChange={setSelectedLevels}
            placeholder="Levels"
          />
          <span className="text-xs text-fg-subtle tabular-nums">
            {filteredLogs.length.toLocaleString()} / {logs.length.toLocaleString()}
          </span>
          <Checkbox
            checked={autoScroll}
            onChange={(e) => setAutoScroll(e.target.checked)}
            label={<span className="text-xs text-fg-muted">Follow</span>}
          />
          <button
            onClick={clearLogs}
            className="px-2 py-0.5 text-xs bg-surface-muted text-fg-muted rounded hover:bg-surface focus-visible:ring-2 focus-visible:ring-focus-ring"
          >
            Clear
          </button>
        </div>
      </div>

      <div
        ref={scrollRef}
        className="mt-3 h-80 overflow-auto rounded border border-border bg-surface font-mono text-sm"
      >
        {filteredLogs.map((log) => (
          <LogRow key={log.id} log={log} />
        ))}
      </div>
    </Card>
  );
}
