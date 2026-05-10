import { useState } from 'react';
import { logLevelClasses } from '../../design/status';

export interface LogEntry {
  id: string;
  timestamp: string;
  level: string;
  module: string;
  message: string;
  attributes: Record<string, unknown>;
}

function formatTime(isoString: string): string {
  const d = new Date(isoString);
  const hh = String(d.getHours()).padStart(2, '0');
  const mm = String(d.getMinutes()).padStart(2, '0');
  const ss = String(d.getSeconds()).padStart(2, '0');
  const ms = String(d.getMilliseconds()).padStart(3, '0');
  return `${hh}:${mm}:${ss}.${ms}`;
}

function formatValue(value: unknown): string {
  if (value === null || value === undefined) return '';
  if (typeof value === 'string') return value;
  if (typeof value === 'number' || typeof value === 'boolean') return String(value);
  return JSON.stringify(value);
}

interface LogRowProps {
  readonly log: LogEntry;
  readonly inlineAttributes?: string[];
}

export function LogRow({ log, inlineAttributes = [] }: LogRowProps) {
  const [expanded, setExpanded] = useState(false);
  const levelColor = logLevelClasses(log.level);
  const allAttrKeys = Object.keys(log.attributes);
  const hiddenAttrKeys = allAttrKeys.filter(k => !inlineAttributes.includes(k));
  const hasHiddenAttributes = hiddenAttrKeys.length > 0;
  const isSystemMarker = log.module === 'system';

  return (
    <div
      style={{
        contentVisibility: 'auto',
        containIntrinsicSize: 'auto 24px',
      }}
      className={`px-3 py-0.5 border-b border-border cursor-default ${
        isSystemMarker
          ? 'bg-warning-soft/50 hover:bg-warning-soft/70'
          : 'hover:bg-surface-muted'
      }`}
      onClick={() => hasHiddenAttributes && setExpanded(!expanded)}
    >
      <div className="flex items-start">
        <span className="text-fg-subtle shrink-0">{formatTime(log.timestamp)}</span>
        <span className={`${levelColor} ml-2 shrink-0`}>[{log.level.toUpperCase().padEnd(5)}]</span>
        <span className="text-info ml-1 shrink-0">[{log.module}]</span>
        {inlineAttributes.map(key => {
          const value = log.attributes[key];
          if (value === undefined) return null;
          return (
            <span key={key} className="ml-2 shrink-0 text-sm">
              <span className="text-canvas-soft-fg">{key}</span>
              <span className="text-fg-subtle">=</span>
              <span className="text-srt-soft-fg">{formatValue(value)}</span>
            </span>
          );
        })}
        <span className="text-fg ml-2 break-all">{log.message}</span>
        {typeof log.attributes['suppressed'] === 'number' && log.attributes['suppressed'] > 0 && (
          <span
            key={String(log.attributes['suppressed'])}
            className="ml-2 shrink-0 self-center px-1.5 rounded-full bg-srt-soft text-srt-soft-fg text-xs leading-normal tabular-nums animate-[blip_100ms_ease-out]"
          >
            x{log.attributes['suppressed']}
          </span>
        )}
        {hasHiddenAttributes && !expanded && (
          <span className="text-fg-subtle ml-2 shrink-0">+{hiddenAttrKeys.length}</span>
        )}
      </div>
      {expanded && hasHiddenAttributes && (
        <div className="ml-[7.5rem] mt-1 mb-1 pl-2 border-l border-border">
          {hiddenAttrKeys.map(key => (
            <div key={key} className="text-sm">
              <span className="text-canvas-soft-fg">{key}</span>
              <span className="text-fg-subtle">=</span>
              <span className="text-srt-soft-fg">{formatValue(log.attributes[key])}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
