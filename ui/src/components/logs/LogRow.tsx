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
}

export function LogRow({ log }: LogRowProps) {
  const [expanded, setExpanded] = useState(false);
  const levelColor = LEVEL_COLORS[log.level] || 'text-blue-400';
  const hasAttributes = Object.keys(log.attributes).length > 0;

  return (
    <div
      style={{
        contentVisibility: 'auto',
        containIntrinsicSize: 'auto 24px',
      }}
      className="px-3 py-0.5 hover:bg-gray-800 border-b border-gray-800/50 cursor-default"
      onClick={() => hasAttributes && setExpanded(!expanded)}
    >
      <div className="flex items-start">
        <span className="text-gray-500 shrink-0">{formatTime(log.timestamp)}</span>
        <span className={`${levelColor} ml-2 shrink-0`}>[{log.level.toUpperCase().padEnd(5)}]</span>
        <span className="text-cyan-400 ml-1 shrink-0">[{log.module}]</span>
        <span className="text-gray-200 ml-2 break-all">{log.message}</span>
        {hasAttributes && !expanded && (
          <span className="text-gray-600 ml-2 shrink-0">+{Object.keys(log.attributes).length}</span>
        )}
      </div>
      {expanded && hasAttributes && (
        <div className="ml-[7.5rem] mt-1 mb-1 pl-2 border-l border-gray-700">
          {Object.entries(log.attributes).map(([key, value]) => (
            <div key={key} className="text-sm">
              <span className="text-purple-400">{key}</span>
              <span className="text-gray-600">=</span>
              <span className="text-yellow-300">{formatValue(value)}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
