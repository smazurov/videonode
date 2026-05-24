// Format a Unix-microseconds start timestamp as a human-readable uptime
// (e.g. "3h12m", "27s"). Returns undefined when the input is missing,
// zero, or in the future — callers can render a dash for that case.
export function formatUptime(startedAtUs?: number): string | undefined {
  if (!startedAtUs || startedAtUs === 0) return undefined;
  const startedMs = Math.floor(startedAtUs / 1000);
  const ageMs = Date.now() - startedMs;
  if (ageMs < 0) return undefined;
  const seconds = Math.floor(ageMs / 1000);
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m${seconds % 60}s`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h${minutes % 60}m`;
  const days = Math.floor(hours / 24);
  return `${days}d${hours % 24}h`;
}
