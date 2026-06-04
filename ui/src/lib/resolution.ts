export function parseResolution(resolution: string | undefined): { width: number; height: number } {
  if (!resolution) return { width: 0, height: 0 };
  const parts = resolution.split('x');
  if (parts.length !== 2) return { width: 0, height: 0 };
  const width = Number.parseInt(parts[0] ?? '0', 10) || 0;
  const height = Number.parseInt(parts[1] ?? '0', 10) || 0;
  return { width, height };
}

const KNOWN_ASPECT_RATIOS: ReadonlyArray<{ label: string; value: number }> = [
  { label: '16:9', value: 16 / 9 },
  { label: '16:10', value: 16 / 10 },
  { label: '4:3', value: 4 / 3 },
  { label: '3:2', value: 3 / 2 },
  { label: '5:4', value: 5 / 4 },
  { label: '21:9', value: 64 / 27 },
  { label: '1:1', value: 1 },
];

function gcd(a: number, b: number): number {
  return b === 0 ? a : gcd(b, a % b);
}

/**
 * Aspect-ratio badge for a resolution, e.g. "16:9". Snaps to a common ratio
 * within 1% tolerance; otherwise returns the reduced ratio when it's tidy,
 * or undefined when it would be noise (e.g. 683:384).
 */
export function aspectRatioLabel(width: number, height: number): string | undefined {
  if (width <= 0 || height <= 0) return undefined;
  const ratio = width / height;
  for (const { label, value } of KNOWN_ASPECT_RATIOS) {
    if (Math.abs(ratio - value) / value <= 0.01) return label;
  }
  const divisor = gcd(width, height);
  const w = width / divisor;
  const h = height / divisor;
  if (w <= 50 && h <= 50) return `${w}:${h}`;
  return undefined;
}
