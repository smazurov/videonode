import { encoderActive, maxRelayMs, type StreamHealth } from './stream-health';

// The relay normally forwards a frame within a few ms of its last packet;
// holding frames this long means WebRTC sends are backing up.
export const RELAY_WARN_MS = 200;
const SPEED_WARN = 0.95;

export type Tone = 'ok' | 'warn' | 'bad' | 'off';

export interface StreamSummary {
  readonly tone: Tone;
  readonly headline: string;
  readonly detail?: string | undefined;
}

/** One pipeline hop as shown on the stream page: a word or two plus a tone. */
export interface StageState {
  readonly state: string;
  readonly tone: Tone;
}

export interface SourceLiveness {
  readonly id: string;
  readonly live: boolean;
  /** Human label for the liveness token, e.g. "No signal". */
  readonly label: string;
}

export function secondsAgo(ms: number | undefined): string {
  const s = Math.round((ms ?? 0) / 1000);
  return s < 60 ? `${s}s` : `${Math.round(s / 60)}m`;
}

function encoderSummary(health: StreamHealth | undefined, lastError: string | undefined): StreamSummary | null {
  const state = health?.encoder.state ?? 'idle';
  if (state === 'error') return { tone: 'bad', headline: 'Encoder crashed', detail: lastError ?? 'Next viewer restarts it' };
  if (state === 'idle') return { tone: 'off', headline: 'Idle', detail: 'Starts when someone watches' };
  if (state === 'stopping') return { tone: 'off', headline: 'Stopping' };
  if (state === 'starting') return { tone: 'warn', headline: 'Starting' };
  return null;
}

function mediaSummary(health: StreamHealth): StreamSummary | null {
  if (health.encoder.ffmpeg.frames.state === 'stalled') return { tone: 'bad', headline: 'Encoder stopped producing frames' };
  const stalled = (health.tracks ?? []).find((t) => t.packets.state === 'stalled');
  if (stalled) {
    const kind = stalled.type === 'audio' ? 'Audio' : 'Video';
    return { tone: 'bad', headline: `${kind} stopped ${secondsAgo(stalled.packets.age_ms)} ago` };
  }
  const relay = maxRelayMs(health) ?? 0;
  if (relay >= RELAY_WARN_MS) return { tone: 'warn', headline: `Relay holding frames ${Math.round(relay)} ms` };
  const speed = health.encoder.ffmpeg.stats.speed;
  if (speed > 0 && speed < SPEED_WARN) return { tone: 'warn', headline: `Encoder can't keep up (${speed.toFixed(2)}×)` };
  return null;
}

/**
 * One sentence for the whole stream. Checks run upstream to downstream so the
 * first problem in the chain is the one reported.
 */
export function summarizeStream(
  connected: boolean,
  health: StreamHealth | undefined,
  sources: readonly SourceLiveness[],
  lastError: string | undefined,
): StreamSummary {
  if (!connected) return { tone: 'off', headline: 'Telemetry disconnected' };
  const encoder = encoderSummary(health, lastError);
  if (encoder || !health) return encoder ?? { tone: 'off', headline: 'Idle' };
  const down = sources.find((s) => !s.live);
  if (down) return { tone: 'warn', headline: `${down.id}: ${down.label.toLowerCase()}` };
  return mediaSummary(health) ?? { tone: 'ok', headline: 'Healthy' };
}

export function encoderStage(health: StreamHealth | undefined, encodedFps: number | null | undefined): StageState {
  if (health?.encoder.state === 'error') return { state: 'crashed', tone: 'bad' };
  if (!health || !encoderActive(health)) return { state: health?.encoder.state ?? 'idle', tone: 'off' };
  if (health.encoder.ffmpeg.frames.state === 'stalled') return { state: 'stalled', tone: 'bad' };
  return { state: encodedFps == null ? 'starting' : `${encodedFps.toFixed(0)} fps`, tone: 'ok' };
}

export function relayStage(health: StreamHealth | undefined): StageState {
  if (!health || !encoderActive(health)) return { state: '—', tone: 'off' };
  const tracks = health.tracks ?? [];
  const stalled = tracks.filter((t) => t.packets.state === 'stalled');
  if (stalled.length > 0) return { state: stalled.map((t) => `${t.type} stopped`).join(', '), tone: 'bad' };
  const state = tracks.map((t) => t.type).join(' + ') || 'waiting';
  return { state, tone: (maxRelayMs(health) ?? 0) >= RELAY_WARN_MS ? 'warn' : 'ok' };
}

/** Network viewers, or "recording" when only the recorder keeps the encoder up. */
export function viewersStage(viewerCount: number, recording: boolean): StageState {
  if (viewerCount > 0) return { state: String(viewerCount), tone: 'ok' };
  if (recording) return { state: 'recording', tone: 'ok' };
  return { state: '0', tone: 'off' };
}

export function composerStage(status: string | undefined): StageState {
  if (status === 'running') return { state: status, tone: 'ok' };
  if (status === 'error') return { state: status, tone: 'bad' };
  return { state: status ?? 'unknown', tone: 'off' };
}
