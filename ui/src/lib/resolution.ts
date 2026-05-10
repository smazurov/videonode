export function parseResolution(resolution: string | undefined): { width: number; height: number } {
  if (!resolution) return { width: 0, height: 0 };
  const parts = resolution.split('x');
  if (parts.length !== 2) return { width: 0, height: 0 };
  const width = Number.parseInt(parts[0] ?? '0', 10) || 0;
  const height = Number.parseInt(parts[1] ?? '0', 10) || 0;
  return { width, height };
}
