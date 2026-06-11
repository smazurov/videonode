import type { components } from '../../lib/api.generated';

type StreamData = components['schemas']['StreamData'];

export function codecBitrate(stream: StreamData): string {
  const codec = stream.encoder?.codec || '—';
  const bitrate = stream.encoder?.bitrate;
  if (!bitrate) return codec.toLowerCase();
  return `${codec.toLowerCase()} ${bitrate}`;
}
