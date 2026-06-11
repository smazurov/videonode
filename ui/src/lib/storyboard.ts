import { fetchTextRaw } from './api_fetch';

// Sprite sheets are written as a 10×10 grid by the recorder
// (internal/streaming/recording_thumbs.go).
export const SPRITE_GRID = 10;

export interface StoryboardCue {
  readonly start: number;
  readonly end: number;
  /** Image URL relative to the session base (no #xywh fragment). */
  readonly url: string;
  /** Sprite tile rect; null for legacy whole-image cues. */
  readonly xywh: { x: number; y: number; w: number; h: number } | null;
}

function parseClock(s: string): number {
  const m = /^(\d+):(\d{2}):(\d{2})\.(\d{3})$/.exec(s.trim());
  if (!m) return NaN;
  return (
    Number(m[1]) * 3600 + Number(m[2]) * 60 + Number(m[3]) + Number(m[4]) / 1000
  );
}

// parseStoryboardVTT parses the recorder's thumbnails.vtt into cues. It
// understands sprite-sheet cues (sprites/sprite_001.jpg#xywh=x,y,w,h) and
// legacy per-frame cues (thumbs/00001.jpg) from sessions recorded before the
// sprite format.
export function parseStoryboardVTT(text: string): StoryboardCue[] {
  const cues: StoryboardCue[] = [];
  const lines = text.split('\n');
  for (let i = 0; i < lines.length - 1; i++) {
    const timing = /^(\S+)\s+-->\s+(\S+)/.exec(lines[i] ?? '');
    if (!timing) continue;
    const start = parseClock(timing[1] ?? '');
    const end = parseClock(timing[2] ?? '');
    const payload = (lines[i + 1] ?? '').trim();
    if (!payload || Number.isNaN(start) || Number.isNaN(end)) continue;
    const [url, fragment] = payload.split('#');
    let xywh: StoryboardCue['xywh'] = null;
    const frag = /^xywh=(\d+),(\d+),(\d+),(\d+)$/.exec(fragment ?? '');
    if (frag) {
      xywh = {
        x: Number(frag[1]),
        y: Number(frag[2]),
        w: Number(frag[3]),
        h: Number(frag[4]),
      };
    }
    if (url) cues.push({ start, end, url, xywh });
    i++;
  }
  return cues;
}

// fetchStoryboard loads and parses a session's thumbnails.vtt; resolves to
// null on any failure (recording just started, file not yet written).
export async function fetchStoryboard(vttUrl: string): Promise<StoryboardCue[] | null> {
  const text = await fetchTextRaw(vttUrl);
  if (text == null) return null;
  return parseStoryboardVTT(text);
}
