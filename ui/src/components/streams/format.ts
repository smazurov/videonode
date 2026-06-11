import type { components } from '../../lib/api.generated';

type StreamData = components['schemas']['StreamData'];

export function codecBitrate(stream: StreamData): string {
  const codec = stream.encoder?.codec || '—';
  const bitrate = stream.encoder?.bitrate;
  if (!bitrate) return codec.toLowerCase();
  return `${codec.toLowerCase()} ${bitrate}`;
}

export function formatClock(sec: number): string {
  const s = Math.max(0, Math.floor(sec));
  const m = Math.floor(s / 60);
  return `${String(m).padStart(2, '0')}:${String(s % 60).padStart(2, '0')}`;
}

export function formatBytes(bytes: number | undefined): string {
  if (!bytes || bytes <= 0) return '—';
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`;
}

export function formatStarted(iso: string | undefined, fallback = '—'): string {
  if (!iso) return fallback;
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? fallback : d.toLocaleString();
}
