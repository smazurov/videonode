import type { components } from '../../lib/api.generated';

type StreamData = Partial<components['schemas']['StreamData']> & { stream_id: string };

export interface UpstreamRef {
  readonly raw: string;
  readonly kind: 'source' | 'composer' | 'unknown';
  readonly id: string;
  readonly href: string | null;
}

export function resolveUpstream(stream: StreamData): UpstreamRef {
  if (typeof stream.upstream === 'string' && stream.upstream.length > 0) {
    return parseUpstream(stream.upstream);
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
