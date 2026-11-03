import type { components } from './api.generated';

export type StreamHealth = components['schemas']['StreamHealth'];
export type ActivityHealth = components['schemas']['ActivityHealth'];

export function latestHealth(
  rest: StreamHealth | undefined,
  pushed: StreamHealth | undefined,
): StreamHealth | undefined {
  if (!rest) return pushed;
  if (!pushed) return rest;
  return Date.parse(rest.sampled_at) > Date.parse(pushed.sampled_at) ? rest : pushed;
}

export function encoderActive(health: StreamHealth): boolean {
  return health.encoder.state === 'running' || health.encoder.state === 'starting';
}

export function ffmpegStatsFresh(health: StreamHealth): boolean {
  return encoderActive(health)
    && health.encoder.ffmpeg.progress.state === 'flowing'
    && health.encoder.ffmpeg.frames.state === 'flowing';
}

// Flow keys identify one counter along the encoder → relay → fanout chain so
// per-second rates can be derived from consecutive health samples.
export const ENCODED_FLOW = 'encoded';
export const packetFlow = (trackIndex: number) => `pkt:${trackIndex}`;
export const unitFlow = (trackIndex: number) => `unit:${trackIndex}`;

export function flowCounts(health: StreamHealth | undefined): Record<string, number> {
  if (!health || !encoderActive(health)) return {};
  const counts: Record<string, number> = { [ENCODED_FLOW]: health.encoder.ffmpeg.frames.count };
  for (const track of health.tracks ?? []) {
    counts[packetFlow(track.index)] = track.packets.count;
    if (track.units.state !== 'unsupported') counts[unitFlow(track.index)] = track.units.count;
  }
  return counts;
}

// Per-second rate for each flow present in both samples; null when there is
// no previous sample or a counter went backwards (publisher restart).
export function flowRates(
  previous: { readonly ts: number; readonly counts: Readonly<Record<string, number>> } | undefined,
  ts: number,
  counts: Readonly<Record<string, number>>,
): Record<string, number | null> {
  const dtSec = previous ? (ts - previous.ts) / 1000 : 0;
  const rates: Record<string, number | null> = {};
  for (const [key, count] of Object.entries(counts)) {
    const before = previous?.counts[key];
    rates[key] = dtSec > 0 && before != null && count >= before ? (count - before) / dtSec : null;
  }
  return rates;
}

// Worst per-track relay lag (frame arriving from the encoder → written to
// WebRTC), or undefined while no WebRTC viewer is receiving.
export function maxRelayMs(health: StreamHealth | undefined): number | undefined {
  let worst: number | undefined;
  for (const track of health?.tracks ?? []) {
    if (track.relay_ms != null && (worst == null || track.relay_ms > worst)) worst = track.relay_ms;
  }
  return worst;
}
