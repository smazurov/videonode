import { useMemo } from 'react';
import { useStreamStore } from '../../hooks/useStreamStore';
import type { StreamConsumers } from '../../hooks/entityTypes';

export interface StreamViewer {
  readonly key: string;
  readonly protocol: 'webrtc' | 'srt' | 'rtsp';
  readonly id: string;
  readonly ip?: string | undefined;
  readonly since?: string | undefined;
  /** Bits per second since the previous consumers push; WebRTC and SRT only. */
  readonly rate?: number | undefined;
  readonly rtt?: number | undefined;
  readonly jitter?: number | undefined;
}

// Per-stream byte counters from the previous stream.consumers push (~1 s
// apart). Module-level so the delta survives re-renders without touching refs
// during render; keyed on the push object so a repeated render of the same
// push returns the same rates instead of a zero delta.
interface RateEntry {
  readonly push: StreamConsumers | undefined;
  readonly bytes: Readonly<Record<string, number>>;
  readonly rates: Readonly<Record<string, number>>;
}
const rateCache = new Map<string, RateEntry>();

function ratesFor(streamId: string, consumers: StreamConsumers | undefined): Readonly<Record<string, number>> {
  const entry = rateCache.get(streamId);
  if (entry && entry.push === consumers) return entry.rates;
  const previous = entry?.bytes ?? {};
  const bytes: Record<string, number> = {};
  const rates: Record<string, number> = {};
  for (const c of [...(consumers?.webrtc_clients ?? []), ...(consumers?.srt_clients ?? [])]) {
    bytes[c.id] = c.bytes_sent;
    const before = previous[c.id];
    if (before != null && c.bytes_sent >= before) rates[c.id] = (c.bytes_sent - before) * 8;
  }
  rateCache.set(streamId, { push: consumers, bytes, rates });
  return rates;
}

/** Network viewers of a stream (WebRTC, SRT, RTSP) from the live consumers push. */
export function useStreamViewers(streamId: string): StreamViewer[] {
  const consumers = useStreamStore((s) => s.consumersById[streamId]);
  return useMemo(() => {
    const rates = ratesFor(streamId, consumers);
    return [
      ...(consumers?.webrtc_clients ?? []).map((c) => ({
        key: `webrtc:${c.id}`, protocol: 'webrtc' as const, id: c.id, ip: c.client_ip,
        since: c.connected_since, rate: rates[c.id], rtt: c.rtt_ms, jitter: c.jitter_ms,
      })),
      ...(consumers?.srt_clients ?? []).map((c) => ({
        key: `srt:${c.id}`, protocol: 'srt' as const, id: c.id, ip: c.client_ip,
        since: c.connected_since, rate: rates[c.id], rtt: c.rtt_ms,
      })),
      ...(consumers?.rtsp_clients ?? []).map((c) => ({
        key: `rtsp:${c.id}`, protocol: 'rtsp' as const, id: c.id, ip: c.client_ip, since: c.connected_since,
      })),
    ];
  }, [streamId, consumers]);
}
