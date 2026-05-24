import type { components } from '../../lib/api.generated';

type StreamData = components['schemas']['StreamData'];

export interface UpstreamRef {
  readonly raw: string;
  readonly kind: 'source' | 'composer' | 'unknown';
  readonly id: string;
  readonly href: string | null;
}

// Resolve a stream's upstream reference. Prefers the (future) typed
// `upstream` field exposed by B7; falls back to deriving from today's
// `canvas`/`device_id` shape so the UI works against either schema.
export function resolveUpstream(stream: StreamData): UpstreamRef {
  const upstream = (stream as { upstream?: string }).upstream;
  if (typeof upstream === 'string' && upstream.length > 0) {
    return parseUpstream(upstream);
  }
  if (stream.canvas) {
    return {
      raw: `composer:${stream.stream_id}`,
      kind: 'composer',
      id: stream.stream_id,
      href: `/composers/${stream.stream_id}`,
    };
  }
  if (stream.device_id) {
    return {
      raw: `source:${stream.device_id}`,
      kind: 'source',
      id: stream.device_id,
      href: `/sources/${stream.device_id}`,
    };
  }
  return { raw: 'unknown', kind: 'unknown', id: '', href: null };
}

function parseUpstream(value: string): UpstreamRef {
  const [kindRaw, ...rest] = value.split(':');
  const id = rest.join(':');
  if (kindRaw === 'source' && id) {
    return { raw: value, kind: 'source', id, href: `/sources/${id}` };
  }
  if (kindRaw === 'composer' && id) {
    return { raw: value, kind: 'composer', id, href: `/composers/${id}` };
  }
  return { raw: value, kind: 'unknown', id, href: null };
}
