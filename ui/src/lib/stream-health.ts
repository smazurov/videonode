import type { components } from './api.generated';

export type StreamHealth = components['schemas']['StreamHealth'];
export type ActivityHealth = components['schemas']['ActivityHealth'];
export type SinkHealth = components['schemas']['SinkHealth'];
export type AudioInputHealth = components['schemas']['AudioInputHealth'];

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
export const REPEATED_FLOW = 'sink:repeated';
export const EARLY_DROP_FLOW = 'sink:early_drop';
export const packetFlow = (trackIndex: number) => `pkt:${trackIndex}`;
export const unitFlow = (trackIndex: number) => `unit:${trackIndex}`;
export const FFMPEG_DUP_FLOW = 'ffmpeg:dup';
export const FFMPEG_DROP_FLOW = 'ffmpeg:drop';
export const silenceFlow = (audioTrack: number) => `silence:${audioTrack}`;
export const xrunFlow = (audioTrack: number) => `xrun:${audioTrack}`;

/** Window for "recent" problems on the stream page: long enough to read, short enough to clear. */
export const RECENT_WINDOW_MS = 10_000;

interface CountSample {
  readonly ts: number;
  readonly counts: Readonly<Record<string, number>>;
}

/**
 * How much a counter grew over the last windowMs of history. A counter that
 * went backwards restarted with a new encoder run, so its current value is
 * all growth since then.
 */
export function recentIncrease(history: readonly CountSample[], flow: string, windowMs = RECENT_WINDOW_MS): number {
  const latest = history[history.length - 1];
  const now = latest?.counts[flow];
  if (latest == null || now == null) return 0;
  let base: number | undefined;
  for (let i = history.length - 2; i >= 0; i -= 1) {
    const sample = history[i]!;
    const count = sample.counts[flow];
    if (count == null) break;
    if (count > now) {
      base = 0;
      break;
    }
    base = count;
    if (latest.ts - sample.ts >= windowMs) break;
  }
  return base == null ? 0 : now - base;
}

/** vn-sink's report for the running encoder, or undefined before its first report (or from an older daemon). */
export function sinkHealth(health: StreamHealth | undefined): SinkHealth | undefined {
  const sink = health?.encoder.sink;
  return health && encoderActive(health) && sink && sink.reports.count > 0 ? sink : undefined;
}

export function degradedAudio(input: AudioInputHealth): boolean {
  return input.state === 'absent' || input.state === 'stalled';
}

export function flowCounts(health: StreamHealth | undefined): Record<string, number> {
  if (!health || !encoderActive(health)) return {};
  const counts: Record<string, number> = { [ENCODED_FLOW]: health.encoder.ffmpeg.frames.count };
  for (const track of health.tracks ?? []) {
    counts[packetFlow(track.index)] = track.packets.count;
    if (track.units.state !== 'unsupported') counts[unitFlow(track.index)] = track.units.count;
  }
  counts[FFMPEG_DUP_FLOW] = health.encoder.ffmpeg.stats.duplicate_frames;
  counts[FFMPEG_DROP_FLOW] = health.encoder.ffmpeg.stats.dropped_frames;
  const sink = sinkHealth(health);
  if (sink) {
    counts[REPEATED_FLOW] = sink.video.repeated_frames;
    counts[EARLY_DROP_FLOW] = sink.video.dropped_frames;
    for (const input of sink.audio ?? []) {
      counts[silenceFlow(input.track)] = input.silence_ms;
      counts[xrunFlow(input.track)] = input.xruns;
    }
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
