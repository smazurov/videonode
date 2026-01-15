import { useState } from 'react';

export interface LogEntry {
  id: string;
  timestamp: string;
  level: string;
  module: string;
  message: string;
  attributes: Record<string, unknown>;
}

const LEVEL_COLORS: Record<string, string> = {
  error: 'text-red-400',
  warn: 'text-yellow-400',
  info: 'text-blue-400',
  debug: 'text-gray-500',
};

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
  const levelColor = LEVEL_COLORS[log.level] || 'text-blue-400';
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
      className={`px-3 py-0.5 border-b border-gray-800/50 cursor-default ${
        isSystemMarker
          ? 'bg-amber-900/30 hover:bg-amber-900/40'
          : 'hover:bg-gray-800'
      }`}
      onClick={() => hasHiddenAttributes && setExpanded(!expanded)}
    >
      <div className="flex items-start">
        <span className="text-gray-500 shrink-0">{formatTime(log.timestamp)}</span>
        <span className={`${levelColor} ml-2 shrink-0`}>[{log.level.toUpperCase().padEnd(5)}]</span>
        <span className="text-cyan-400 ml-1 shrink-0">[{log.module}]</span>
        {inlineAttributes.map(key => {
          const value = log.attributes[key];
          if (value === undefined) return null;
          return (
            <span key={key} className="ml-2 shrink-0 text-sm">
              <span className="text-purple-400">{key}</span>
              <span className="text-gray-600">=</span>
              <span className="text-yellow-300">{formatValue(value)}</span>
            </span>
          );
        })}
        <span className="text-gray-200 ml-2 break-all">{log.message}</span>
        {hasHiddenAttributes && !expanded && (
          <span className="text-gray-600 ml-2 shrink-0">+{hiddenAttrKeys.length}</span>
        )}
      </div>
      {expanded && hasHiddenAttributes && (
        <div className="ml-[7.5rem] mt-1 mb-1 pl-2 border-l border-gray-700">
          {hiddenAttrKeys.map(key => (
            <div key={key} className="text-sm">
              <span className="text-purple-400">{key}</span>
              <span className="text-gray-600">=</span>
              <span className="text-yellow-300">{formatValue(log.attributes[key])}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
