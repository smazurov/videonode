import { describe, expect, it } from 'vitest';
import type { ActivityHealth, AudioInputHealth, SinkHealth, StreamHealth } from './stream-health';
import {
  audioInputStage,
  clockTime,
  formatDuration,
  encoderStage,
  relayStage,
  summarizeStream,
  upstreamFramesLine,
  viewersStage,
  type SourceLiveness,
} from './stream-status';

const SAMPLED_AT = '2026-06-01T00:00:00Z';
const SAMPLED_US = Date.parse(SAMPLED_AT) * 1000;

const flowing = (count: number): ActivityHealth => ({ state: 'flowing', count, age_ms: 10 });
const stalled = (count: number): ActivityHealth => ({ state: 'stalled', count, age_ms: 30_000 });

function health(state: StreamHealth['encoder']['state'] = 'running'): StreamHealth {
  return {
    sampled_at: SAMPLED_AT,
    stall_after_ms: 5000,
    publisher_started_at_us: 123,
    encoder: {
      state,
      started_at_us: 123,
      ffmpeg: {
        progress: flowing(10),
        frames: flowing(300),
        stats: { fps: 30, speed: 1, dropped_frames: 0, duplicate_frames: 0 },
      },
    },
    tracks: [
      { index: 0, type: 'video', codec: 'H264', packets: flowing(2000), units: flowing(300), relay_ms: 3 },
      { index: 1, type: 'audio', codec: 'opus', packets: flowing(500), units: flowing(500), relay_ms: 1 },
    ],
  };
}

function lyra(state: AudioInputHealth['state'], extra: Partial<AudioInputHealth> = {}): AudioInputHealth {
  return {
    track: 0, device: 'vncap:CARD=Lyra,DEV=0', state, since_us: SAMPLED_US - 12_000_000,
    xruns: 0, silence_ms: 0, dropped_ms: 0, ...extra,
  };
}

function withSink(h: StreamHealth, audio: AudioInputHealth[], video: Partial<SinkHealth['video']> = {}): StreamHealth {
  h.encoder.sink = {
    reports: flowing(5),
    video: { upstream: 'connected', repeated_frames: 0, dropped_frames: 0, ...video },
    audio,
  };
  return h;
}

const VIDEO_AUDIO = 'video + audio';

const live: SourceLiveness = { id: 'lyra', live: true, label: 'Live' };
const noSignal: SourceLiveness = { id: 'lyra', live: false, label: 'No signal' };

describe('summarizeStream', () => {
  it('is healthy when everything flows', () => {
    expect(summarizeStream(true, health(), [live], undefined)).toEqual({ tone: 'ok', headline: 'Healthy' });
  });

  it('reports lost telemetry before guessing at health', () => {
    expect(summarizeStream(false, health('error'), [noSignal], undefined).headline).toBe('Telemetry disconnected');
  });

  it('explains an idle encoder as lazy, not broken', () => {
    expect(summarizeStream(true, health('idle'), [live], undefined)).toMatchObject({ tone: 'off', headline: 'Idle' });
    expect(summarizeStream(true, undefined, [], undefined)).toMatchObject({ tone: 'off', headline: 'Idle' });
  });

  it('surfaces a crashed encoder with the supervisor error', () => {
    expect(summarizeStream(true, health('error'), [live], 'process exited with code 1')).toEqual({
      tone: 'bad', headline: 'Encoder crashed', detail: 'process exited with code 1',
    });
  });

  it('names the first dead source before downstream symptoms', () => {
    const h = health();
    h.tracks[1] = { ...h.tracks[1]!, packets: stalled(500) };
    expect(summarizeStream(true, h, [noSignal], undefined)).toEqual({ tone: 'warn', headline: 'lyra: no signal' });
  });

  it('reports which track stopped and when', () => {
    const h = health();
    h.tracks[1] = { ...h.tracks[1]!, packets: stalled(500) };
    expect(summarizeStream(true, h, [live], undefined)).toEqual({ tone: 'bad', headline: 'Audio stopped 30s ago' });
  });

  it('warns when the relay holds frames before WebRTC', () => {
    const h = health();
    h.tracks[0] = { ...h.tracks[0]!, relay_ms: 350.4 };
    expect(summarizeStream(true, h, [live], undefined)).toEqual({ tone: 'warn', headline: 'Relay holding frames 350 ms' });
    expect(relayStage(h).tone).toBe('warn');
  });

  it('warns when the encoder cannot keep up', () => {
    const h = health();
    h.encoder.ffmpeg.stats.speed = 0.5;
    expect(summarizeStream(true, h, [live], undefined).headline).toBe("Encoder can't keep up (0.50×)");
  });
});

describe('vn-sink status in the summary', () => {
  it('names a missing audio device by its configured name and how long silence has been sent', () => {
    const h = withSink(health(), [lyra('absent', { error: 'No such device', silence_ms: 12_000 })]);
    expect(summarizeStream(true, h, [live], undefined, { audioNames: ['hw:Lyra,0'] })).toEqual({
      tone: 'bad', headline: 'hw:Lyra,0: audio device missing', detail: 'sending silence for 12s',
    });
  });

  it('flags a device that stopped sending samples, falling back to the sink device name', () => {
    const h = withSink(health(), [lyra('stalled')]);
    expect(summarizeStream(true, h, [live], undefined).headline).toBe('vncap:CARD=Lyra,DEV=0: audio device stopped sending');
  });

  it('reports a lost video upstream before audio trouble', () => {
    const h = withSink(health(), [lyra('absent')], { upstream: 'reconnecting' });
    expect(summarizeStream(true, h, [live], undefined, { audioNames: ['hw:Lyra,0'] })).toEqual({
      tone: 'warn', headline: 'Encoder lost its video upstream', detail: 'repeating the last frame',
    });
  });

  it('says when upstream stopped sending frames while the sink repeats them', () => {
    const h = withSink(health(), [lyra('absent')]);
    expect(summarizeStream(true, h, [live], undefined, { repeatedFps: 29.5 })).toEqual({
      tone: 'warn', headline: 'Upstream stopped sending frames', detail: 'encoder is repeating the last one',
    });
    expect(summarizeStream(true, h, [live], undefined, { repeatedFps: 0 }).headline).toBe(
      'vncap:CARD=Lyra,DEV=0: audio device missing',
    );
  });

  it('keeps a dead source ahead of sink symptoms', () => {
    const h = withSink(health(), [lyra('absent')]);
    expect(summarizeStream(true, h, [noSignal], undefined).headline).toBe('lyra: no signal');
  });

  it('stays healthy with live audio, and ignores a missing or unreported sink section', () => {
    expect(summarizeStream(true, withSink(health(), [lyra('live')]), [live], undefined).tone).toBe('ok');
    const unreported = withSink(health(), [lyra('absent')]);
    unreported.encoder.sink!.reports = { state: 'never_received', count: 0 };
    expect(summarizeStream(true, unreported, [live], undefined).tone).toBe('ok');
    expect(summarizeStream(true, health(), [live], undefined).tone).toBe('ok');
  });

  it('ignores a stale sink report once the encoder is idle', () => {
    const h = withSink(health('idle'), [lyra('absent')]);
    expect(summarizeStream(true, h, [live], undefined).headline).toBe('Idle');
  });
});

describe('pipeline stages', () => {
  it('shows encoder rate, idle, and crash', () => {
    expect(encoderStage(health(), 30)).toEqual({ state: '30 fps', tone: 'ok' });
    expect(encoderStage(health('idle'), null)).toEqual({ state: 'idle', tone: 'off' });
    expect(encoderStage(health('error'), null)).toEqual({ state: 'crashed', tone: 'bad' });
  });

  it('marks the encoder while it repeats frames or has lost its upstream', () => {
    expect(encoderStage(withSink(health(), []), 30, { repeatedFps: 4 })).toEqual({ state: '30 fps · repeating', tone: 'warn' });
    expect(encoderStage(withSink(health(), []), 30, { repeatedFps: 0 })).toEqual({ state: '30 fps', tone: 'ok' });
    const lost = withSink(health(), [], { upstream: 'reconnecting' });
    expect(encoderStage(lost, 30, { repeatedFps: 30 })).toEqual({ state: 'no upstream', tone: 'warn' });
  });

  it('shows the encoder and relay carrying silence when a device is missing', () => {
    const h = withSink(health(), [lyra('absent')]);
    expect(encoderStage(h, 30, { repeatedFps: 0 })).toEqual({ state: '30 fps · silent', tone: 'bad' });
    expect(relayStage(h)).toEqual({ state: 'video + silence', tone: 'bad' });
    expect(encoderStage(h, 30, { repeatedFps: 3 })).toEqual({ state: '30 fps · repeating', tone: 'warn' });
    const fine = withSink(health(), [lyra('live')]);
    expect(encoderStage(fine, 30, { repeatedFps: 0 })).toEqual({ state: '30 fps', tone: 'ok' });
    expect(relayStage(fine)).toEqual({ state: VIDEO_AUDIO, tone: 'ok' });
  });

  it('labels each audio track from its own device, or the mix when devices share a track', () => {
    const two = health();
    two.tracks.push({ index: 2, type: 'audio', codec: 'opus', packets: flowing(500), units: flowing(500), relay_ms: 1 });
    withSink(two, [lyra('live'), lyra('absent', { track: 1, device: 'hw:Nope,0' })]);
    expect(relayStage(two)).toEqual({ state: 'video + 2 audio', tone: 'bad' });
    expect(encoderStage(two, 30, { repeatedFps: 0 }).state).toBe('30 fps · 1/2 silent');

    const mixed = withSink(health(), [lyra('live'), lyra('absent', { track: 1, device: 'hw:Nope,0' })]);
    expect(relayStage(mixed)).toEqual({ state: VIDEO_AUDIO, tone: 'bad' });
    const allGone = withSink(health(), [lyra('absent'), lyra('stalled', { track: 1, device: 'hw:Nope,0' })]);
    expect(relayStage(allGone).state).toBe('video + silence');
  });

  it('gives a missing device its reason and start time, and nothing else', () => {
    const input = lyra('absent', { error: 'No such device', silence_ms: 9_400, xruns: 1 });
    expect(audioInputStage(input, SAMPLED_US)).toEqual({
      state: 'missing', tone: 'bad', title: `No such device · since ${clockTime(SAMPLED_US - 12_000_000)}`,
    });
    expect(audioInputStage(lyra('stalled'), SAMPLED_US)).toEqual({
      state: 'no samples', tone: 'bad', title: `since ${clockTime(SAMPLED_US - 12_000_000)}`,
    });
  });

  it('leaves a clean live device without a tooltip', () => {
    expect(audioInputStage(lyra('live', { rate_hz: 48000.94 }), SAMPLED_US)).toEqual({ state: 'live', tone: 'ok' });
  });

  it('lists only the problems this run had, since the run started', () => {
    const input = lyra('live', { xruns: 3, silence_ms: 151_300, dropped_ms: 40 });
    expect(audioInputStage(input, SAMPLED_US).title).toBe(`since ${clockTime(SAMPLED_US)}: 3 xruns · 2m31s silence`);
  });

  it('flags recent dropouts on the chip and puts the counts in the tooltip', () => {
    const input = lyra('live', { xruns: 3, silence_ms: 151_300 });
    expect(audioInputStage(input, SAMPLED_US, { xruns: 1, silenceMs: 400 })).toEqual({
      state: 'dropouts',
      tone: 'warn',
      title: `last 10s: 1 xrun · 0.4s silence\nsince ${clockTime(SAMPLED_US)}: 3 xruns · 2m31s silence`,
    });
  });

  it('formats durations with tenths only while they matter', () => {
    expect(formatDuration(400)).toBe('0.4s');
    expect(formatDuration(23_400)).toBe('23s');
    expect(formatDuration(151_300)).toBe('2m31s');
    expect(formatDuration(3_725_000)).toBe('1h02m');
  });

  it('summarizes repeated and early-dropped frames for the run', () => {
    const h = withSink(health(), [], { repeated_frames: 8, dropped_frames: 1 });
    expect(upstreamFramesLine(h)).toBe('vn-sink: 8 repeated · 1 early');
    expect(upstreamFramesLine(health())).toBeUndefined();
  });

  it('lists relay tracks and flags a stopped one', () => {
    expect(relayStage(health())).toEqual({ state: VIDEO_AUDIO, tone: 'ok' });
    const h = health();
    h.tracks[1] = { ...h.tracks[1]!, packets: stalled(500) };
    expect(relayStage(h)).toEqual({ state: 'audio stopped', tone: 'bad' });
    expect(relayStage(health('idle'))).toEqual({ state: '—', tone: 'off' });
  });

  it('counts viewers, or says recording when only the recorder is reading', () => {
    expect(viewersStage(2, true)).toEqual({ state: '2', tone: 'ok' });
    expect(viewersStage(0, true)).toEqual({ state: 'recording', tone: 'ok' });
    expect(viewersStage(0, false)).toEqual({ state: '0', tone: 'off' });
  });
});
