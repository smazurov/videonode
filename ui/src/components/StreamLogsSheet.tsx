import { useEffect, useMemo, useRef, useState } from 'react';
import { useLogStream } from '../hooks/useLogStream';
import { BottomSheet } from './BottomSheet';
import { Checkbox } from './Checkbox';
import { LogRow } from './logs/LogRow';
import { ALL_LEVELS } from './logs/LogFilters';
import { MultiSelect, type MultiSelectOption } from './MultiSelect';
import { logLevelClasses, connectionStatusClasses } from '../design/status';

export interface StreamLogsSheetProps {
  isOpen: boolean;
  onClose: () => void;
  streamId: string;
}

const LEVEL_OPTIONS: MultiSelectOption[] = ALL_LEVELS.map(level => ({
  value: level,
  label: level.toUpperCase(),
  color: logLevelClasses(level),
}));

function StreamLogsContent({ streamId }: { readonly streamId: string }) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const [autoScroll, setAutoScroll] = useState(true);
  const [selectedLevels, setSelectedLevels] = useState<string[]>(ALL_LEVELS);

  const { logs, connectionStatus, clearLogs } = useLogStream({
    enabled: true,
    maxLogs: 2_000,
  });

  const filteredLogs = useMemo(() =>
    logs.filter(log => {
      if (selectedLevels.length < ALL_LEVELS.length && !selectedLevels.includes(log.level)) return false;
      const sid = log.attributes['stream_id'];
      return typeof sid === 'string' && sid === streamId;
    }),
    [logs, streamId, selectedLevels]
  );

  useEffect(() => {
    if (autoScroll && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [filteredLogs.length, autoScroll]);

  const statusColor = connectionStatusClasses(connectionStatus);

  return (
    <>
      <div className="flex items-center justify-between px-4 pt-2 pb-2 shrink-0">
        <div className="flex items-center gap-1.5">
          <div className={`w-2 h-2 rounded-full ${statusColor}`} />
          <span className="text-xs text-fg-muted">{connectionStatus}</span>
        </div>
        <div className="flex items-center gap-3">
          <MultiSelect
            options={LEVEL_OPTIONS}
            selected={selectedLevels}
            onChange={setSelectedLevels}
            placeholder="Levels"
          />
          <span className="text-xs text-fg-subtle">
            {filteredLogs.length.toLocaleString()} / {logs.length.toLocaleString()}
          </span>
          <Checkbox
            checked={autoScroll}
            onChange={e => setAutoScroll(e.target.checked)}
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

      <div ref={scrollRef} className="flex-1 overflow-auto font-mono text-sm min-h-0">
        {filteredLogs.map(log => (
          <LogRow key={log.id} log={log} />
        ))}
      </div>

      <div className="h-3 shrink-0" />
    </>
  );
}

export function StreamLogsSheet({ isOpen, onClose, streamId }: Readonly<StreamLogsSheetProps>) {
  return (
    <BottomSheet
      open={isOpen}
      onClose={onClose}
      title={`Logs - ${streamId}`}
      maxWidth="5xl"
      maxHeight="max-h-[80vh]"
      padding={false}
    >
      <StreamLogsContent streamId={streamId} />
    </BottomSheet>
  );
}
