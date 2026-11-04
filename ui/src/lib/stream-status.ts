import {
  degradedAudio,
  encoderActive,
  maxRelayMs,
  sinkHealth,
  type AudioInputHealth,
  type StreamHealth,
} from './stream-health';

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

/** A side input feeding a stage, e.g. an audio device captured by the encoder. */
export interface InputState extends StageState {
  /** Hover text, only when it adds something the visible state does not. */
  readonly title?: string;
}

/** Growth of one audio device's problem counters over the recent window. */
export interface AudioRecent {
  readonly xruns: number;
  readonly silenceMs: number;
}

/** What went wrong in the last RECENT_WINDOW_MS, from counter growth in the metrics history. */
export interface RecentEvents {
  readonly repeated: number;
  readonly early: number;
  readonly duplicated: number;
  readonly encoderDropped: number;
  /** By audio track. */
  readonly audio: readonly AudioRecent[];
}

export const NO_RECENT: RecentEvents = { repeated: 0, early: 0, duplicated: 0, encoderDropped: 0, audio: [] };

export function audioDropouts(recent: AudioRecent | undefined): boolean {
  return !!recent && (recent.xruns > 0 || recent.silenceMs > 0);
}

function plural(n: number, word: string): string {
  return `${n} ${word}${n === 1 ? '' : 's'}`;
}

/** 0.4s, 23s, 2m31s, 1h02m: tenths only while they matter. */
export function formatDuration(ms: number): string {
  const s = ms / 1000;
  if (s < 10) return `${s.toFixed(1)}s`;
  if (s < 60) return `${Math.round(s)}s`;
  const total = Math.round(s);
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  if (h > 0) return `${h}h${String(m).padStart(2, '0')}m`;
  return `${m}m${String(total % 60).padStart(2, '0')}s`;
}

/** e.g. "3 xruns · 0.4s silence": only the parts that happened. */
export function dropoutText(counts: AudioRecent): string {
  const parts: string[] = [];
  if (counts.xruns > 0) parts.push(plural(counts.xruns, 'xrun'));
  if (counts.silenceMs > 0) parts.push(`${formatDuration(counts.silenceMs)} silence`);
  return parts.join(' · ');
}

/** Local wall-clock HH:MM for a Unix-microsecond time. */
export function clockTime(us: number): string {
  const d = new Date(us / 1000);
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
}

/** What the stream page knows beyond one health sample. */
export interface SummaryContext {
  /** Configured ALSA device names, by audio track. */
  readonly audioNames?: readonly string[];
  /** Frames per second vn-sink repeated over the last sample interval. */
  readonly repeatedFps?: number | null | undefined;
  readonly recent?: RecentEvents;
}

export interface SourceLiveness {
  readonly id: string;
  readonly live: boolean;
  /** Human label for the liveness token, e.g. "No signal". */
  readonly label: string;
}

export function secondsAgo(ms: number | undefined): string {
  const s = Math.round((ms ?? 0) / 1000);
  if (s < 60) return `${s}s`;
  if (s < 3600) return `${Math.round(s / 60)}m`;
  return `${Math.floor(s / 3600)}h${String(Math.floor((s % 3600) / 60)).padStart(2, '0')}m`;
}

// "12s": how long an audio device has been in its state, as of the health
// sample; empty when the sink has not reported a time yet.
function stateAge(input: AudioInputHealth, sampledAt: string): string {
  if (!input.since_us) return '';
  return secondsAgo(Date.parse(sampledAt) - input.since_us / 1000);
}

function forDuration(input: AudioInputHealth, sampledAt: string): string {
  const age = stateAge(input, sampledAt);
  return age ? ' for ' + age : '';
}

function audioName(input: AudioInputHealth, audioNames: readonly string[]): string {
  return audioNames[input.track] ?? input.device;
}

// vn-sink substitutes silence and repeats frames itself, so the relay still
// sees media flowing; only the sink's report shows what is really missing.
function sinkSummary(health: StreamHealth, ctx: SummaryContext): StreamSummary | null {
  const sink = sinkHealth(health);
  if (!sink) return null;
  if (sink.video.upstream === 'reconnecting') {
    return { tone: 'warn', headline: 'Encoder lost its video upstream', detail: 'repeating the last frame' };
  }
  if (ctx.repeatedFps) {
    return { tone: 'warn', headline: 'Upstream stopped sending frames', detail: 'encoder is repeating the last one' };
  }
  const audioNames = ctx.audioNames ?? [];
  const inputs = sink.audio ?? [];
  const down = inputs.find(degradedAudio);
  if (down) {
    const what = down.state === 'absent' ? 'audio device missing' : 'audio device stopped sending';
    return {
      tone: 'bad',
      headline: `${audioName(down, audioNames)}: ${what}`,
      detail: 'sending silence' + forDuration(down, health.sampled_at),
    };
  }
  const recentAudio = ctx.recent?.audio ?? [];
  const glitchy = inputs.find((i) => audioDropouts(recentAudio[i.track]));
  if (!glitchy) return null;
  return {
    tone: 'warn',
    headline: `${audioName(glitchy, audioNames)}: audio dropouts`,
    detail: `${dropoutText(recentAudio[glitchy.track]!)} in the last 10s`,
  };
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
  ctx: SummaryContext = {},
): StreamSummary {
  if (!connected) return { tone: 'off', headline: 'Telemetry disconnected' };
  const encoder = encoderSummary(health, lastError);
  if (encoder || !health) return encoder ?? { tone: 'off', headline: 'Idle' };
  const down = sources.find((s) => !s.live);
  if (down) return { tone: 'warn', headline: `${down.id}: ${down.label.toLowerCase()}` };
  return sinkSummary(health, ctx) ?? mediaSummary(health) ?? { tone: 'ok', headline: 'Healthy' };
}

export function encoderStage(
  health: StreamHealth | undefined,
  encodedFps: number | null | undefined,
  ctx: Pick<SummaryContext, 'repeatedFps' | 'recent'> = {},
): StageState {
  if (health?.encoder.state === 'error') return { state: 'crashed', tone: 'bad' };
  if (!health || !encoderActive(health)) return { state: health?.encoder.state ?? 'idle', tone: 'off' };
  if (health.encoder.ffmpeg.frames.state === 'stalled') return { state: 'stalled', tone: 'bad' };
  if (sinkHealth(health)?.video.upstream === 'reconnecting') return { state: 'no upstream', tone: 'warn' };
  if (encodedFps == null) return { state: 'starting', tone: 'ok' };
  const fps = `${encodedFps.toFixed(0)} fps`;
  if (ctx.repeatedFps) return { state: `${fps} · repeating`, tone: 'warn' };
  const inputs = sinkHealth(health)?.audio ?? [];
  const silent = inputs.filter(degradedAudio).length;
  if (silent > 0) {
    const which = silent === inputs.length ? 'silent' : String(silent) + '/' + String(inputs.length) + ' silent';
    return { state: `${fps} · ${which}`, tone: 'bad' };
  }
  const recentAudio = ctx.recent?.audio ?? [];
  if (inputs.some((i) => audioDropouts(recentAudio[i.track]))) return { state: `${fps} · dropouts`, tone: 'warn' };
  return { state: fps, tone: 'ok' };
}

// What the relay's audio really is. One published track reads "silence" when
// every device feeding it is degraded (with a mix filter several devices feed
// one track); several tracks collapse to a count, and the audio lane beside
// the encoder shows which device is out.
function audioRelayLabel(health: StreamHealth, audioTracks: number): { label: string; degraded: boolean } {
  const inputs = sinkHealth(health)?.audio ?? [];
  const silent = inputs.filter(degradedAudio).length;
  const degraded = silent > 0;
  if (audioTracks > 1) return { label: `${audioTracks} audio`, degraded };
  return { label: degraded && silent === inputs.length ? 'silence' : 'audio', degraded };
}

const AUDIO_STATE: Record<AudioInputHealth['state'], StageState> = {
  live: { state: 'live', tone: 'ok' },
  stalled: { state: 'no samples', tone: 'bad' },
  absent: { state: 'missing', tone: 'bad' },
  opening: { state: 'opening', tone: 'off' },
};

// Degraded: why, and since when. Otherwise the last 10s and this encoder
// run's problems, if any; a clean device has nothing to add to its chip.
function audioTitle(input: AudioInputHealth, runStartedUs: number, recent?: AudioRecent): string | undefined {
  if (degradedAudio(input)) {
    const since = input.since_us ? `since ${clockTime(input.since_us)}` : '';
    return [input.error, since].filter(Boolean).join(' · ') || undefined;
  }
  const lines: string[] = [];
  if (recent && audioDropouts(recent)) lines.push(`last 10s: ${dropoutText(recent)}`);
  const totals = dropoutText({ xruns: input.xruns, silenceMs: input.silence_ms });
  if (totals) lines.push(runStartedUs > 0 ? `since ${clockTime(runStartedUs)}: ${totals}` : totals);
  return lines.length > 0 ? lines.join('\n') : undefined;
}

/** One audio device as vn-sink captures it. */
export function audioInputStage(input: AudioInputHealth, runStartedUs: number, recent?: AudioRecent): InputState {
  const title = audioTitle(input, runStartedUs, recent);
  const base: InputState =
    input.state === 'live' && recent && audioDropouts(recent) ? { state: 'dropouts', tone: 'warn' } : AUDIO_STATE[input.state];
  return title ? { ...base, title } : base;
}

/** A recent pipeline event worth a glance, shown only while it keeps happening. */
export interface EventChip {
  readonly key: string;
  readonly label: string;
  readonly tone: Tone;
  readonly title: string;
}

/** Chips for video events in the recent window; empty when nothing happened. */
export function recentEventChips(recent: RecentEvents): EventChip[] {
  const chips: EventChip[] = [];
  if (recent.repeated > 0) {
    chips.push({
      key: 'repeated', label: `${recent.repeated} repeated`, tone: 'warn',
      title: 'Frames vn-sink repeated because the upstream sent no new frame in time.',
    });
  }
  if (recent.encoderDropped > 0) {
    chips.push({
      key: 'encoder-dropped', label: `${recent.encoderDropped} dropped by ffmpeg`, tone: 'off',
      title: 'Frames ffmpeg dropped to hold the stream frame rate: two arrived within one frame interval.',
    });
  }
  if (recent.duplicated > 0) {
    chips.push({
      key: 'duplicated', label: `${recent.duplicated} duplicated`, tone: 'off',
      title: 'Frames ffmpeg duplicated to hold the output rate: the source delivered fewer frames than the stream fps.',
    });
  }
  if (recent.early > 0) {
    chips.push({
      key: 'early', label: `${recent.early} early dropped`, tone: 'off',
      title: 'Frames vn-sink dropped for arriving faster than the stream fps: the source runs a little faster than configured.',
    });
  }
  return chips;
}

/** vn-sink's run totals: frames repeated over upstream gaps and dropped for arriving early. */
export function upstreamFramesLine(health: StreamHealth | undefined): string | undefined {
  const sink = sinkHealth(health);
  if (!sink) return undefined;
  return `vn-sink: ${sink.video.repeated_frames} repeated · ${sink.video.dropped_frames} early`;
}

export function relayStage(health: StreamHealth | undefined): StageState {
  if (!health || !encoderActive(health)) return { state: '—', tone: 'off' };
  const tracks = health.tracks ?? [];
  const stalled = tracks.filter((t) => t.packets.state === 'stalled');
  if (stalled.length > 0) return { state: stalled.map((t) => `${t.type} stopped`).join(', '), tone: 'bad' };
  const audioTracks = tracks.filter((t) => t.type === 'audio').length;
  const audio = audioRelayLabel(health, audioTracks);
  const kinds = [...tracks.filter((t) => t.type !== 'audio').map((t) => t.type), ...(audioTracks > 0 ? [audio.label] : [])];
  const state = kinds.join(' + ') || 'waiting';
  if (audio.degraded) return { state, tone: 'bad' };
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
