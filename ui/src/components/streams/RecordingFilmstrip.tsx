import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { fetchBitmapRaw } from '../../lib/api_fetch';
import { fetchStoryboard, type StoryboardCue } from '../../lib/storyboard';
import { cn } from '../../utils';

// Tiles narrower than this are unrecognizable slivers; the strip shows fewer,
// wider frames instead (the NLE model: fixed tile width, density per zoom).
const MIN_TILE_PX = 80;
const STRIP_HEIGHT = 48;

interface RecordingFilmstripProps {
  /** Session base URL (no trailing slash), e.g. /api/streams/x/recordings/<session>. */
  readonly baseUrl: string;
  /** Absolute URL of thumbnails.vtt (the storyboard source of truth). */
  readonly vttUrl: string;
  /** Whether to keep polling (recording is still growing). */
  readonly live: boolean;
  /** Current playback position in seconds, drives the playhead overlay. */
  readonly currentTime: number;
  /** Seek the player to the given offset in seconds. */
  readonly onSeek: (seconds: number) => void;
  readonly className?: string;
}

function formatClock(sec: number): string {
  const s = Math.max(0, Math.floor(sec));
  const m = Math.floor(s / 60);
  return `${String(m).padStart(2, '0')}:${String(s % 60).padStart(2, '0')}`;
}

// RecordingFilmstrip is the storyboard timeline, canvas-rendered from sprite
// sheets: tiles share the full width equally and center-crop as they narrow,
// long recordings sample fewer/wider frames (≥MIN_TILE_PX), and the strip
// doubles as the seek surface — click or drag anywhere to scrub, with a
// playhead tracking playback. While live it re-polls the VTT so new frames
// appear as they are written.
export function RecordingFilmstrip({
  baseUrl,
  vttUrl,
  live,
  currentTime,
  onSeek,
  className,
}: RecordingFilmstripProps) {
  const [cues, setCues] = useState<StoryboardCue[]>([]);
  const [width, setWidth] = useState(0);
  const wrapRef = useRef<HTMLDivElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const draggingRef = useRef(false);
  // Sheets keyed by base URL; a changed src (live cache-bust) replaces and
  // closes the old decode so a long live session doesn't pile up bitmaps.
  const bitmapsRef = useRef(new Map<string, { src: string; bmp: ImageBitmap }>());

  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      const next = await fetchStoryboard(vttUrl);
      if (!cancelled && next && next.length > 0) setCues(next);
    };
    void load();
    if (!live) return;
    const timer = window.setInterval(() => void load(), 5000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [vttUrl, live]);

  // Keyed on hasCues: the strip renders nothing until cues arrive, so the
  // wrapper to observe only exists after the first successful VTT load.
  const hasCues = cues.length > 0;
  useEffect(() => {
    if (!hasCues) return;
    const el = wrapRef.current;
    if (!el) return;
    const ro = new ResizeObserver(() => setWidth(el.clientWidth));
    ro.observe(el);
    setWidth(el.clientWidth);
    return () => ro.disconnect();
  }, [hasCues]);

  const interval = useMemo(() => {
    const a = cues[0];
    const b = cues[1];
    return a && b ? Math.max(0.1, b.start - a.start) : 5;
  }, [cues]);
  const span = cues.length > 0 ? (cues[cues.length - 1]?.start ?? 0) + interval : 0;

  // Sampled tiles: as many as fit at ≥MIN_TILE_PX, evenly strided over the cues.
  const tiles = useMemo(() => {
    if (cues.length === 0 || width <= 0) return [];
    const maxTiles = Math.max(1, Math.floor(width / MIN_TILE_PX));
    const count = Math.min(cues.length, maxTiles);
    const out: StoryboardCue[] = [];
    for (let i = 0; i < count; i++) {
      const cue = cues[Math.floor((i * cues.length) / count)];
      if (cue) out.push(cue);
    }
    return out;
  }, [cues, width]);

  // Draw the strip. Sheets are fetched once each (immutable once full); the
  // growing last sheet is cache-busted by cue count while live.
  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas || tiles.length === 0 || width <= 0) return;
    let cancelled = false;

    const lastSheetUrl = cues[cues.length - 1]?.url;
    const srcFor = (cue: StoryboardCue): string => {
      const abs = `${baseUrl}/${cue.url}`;
      return live && cue.url === lastSheetUrl ? `${abs}?v=${cues.length}` : abs;
    };

    const draw = async () => {
      const dpr = window.devicePixelRatio || 1;
      const cssH = STRIP_HEIGHT;
      canvas.width = Math.round(width * dpr);
      canvas.height = Math.round(cssH * dpr);
      const ctx = canvas.getContext('2d');
      if (!ctx) return;

      const cache = bitmapsRef.current;
      const wanted = new Map(tiles.map((c) => [c.url, srcFor(c)]));
      await Promise.all(
        [...wanted].map(async ([key, src]) => {
          if (cache.get(key)?.src === src) return;
          const bmp = await fetchBitmapRaw(src);
          if (!bmp) return; // transient (sheet mid-write); next poll repaints
          cache.get(key)?.bmp.close();
          cache.set(key, { src, bmp });
        }),
      );
      if (cancelled) return;

      ctx.clearRect(0, 0, canvas.width, canvas.height);
      const dw = (width / tiles.length) * dpr;
      const dh = cssH * dpr;
      tiles.forEach((cue, i) => {
        const bmp = cache.get(cue.url)?.bmp;
        if (!bmp) return;
        const sx = cue.xywh?.x ?? 0;
        const sy = cue.xywh?.y ?? 0;
        const sw = cue.xywh?.w ?? bmp.width;
        const sh = cue.xywh?.h ?? bmp.height;
        // object-cover: crop the source horizontally to the tile's aspect.
        const wantW = Math.min(sw, (dw / dh) * sh);
        ctx.drawImage(bmp, sx + (sw - wantW) / 2, sy, wantW, sh, i * dw, 0, dw, dh);
      });
    };
    void draw();
    return () => {
      cancelled = true;
    };
  }, [tiles, cues, width, baseUrl, live]);

  const seekFromPointer = useCallback(
    (clientX: number) => {
      const el = wrapRef.current;
      if (!el || span <= 0) return;
      const rect = el.getBoundingClientRect();
      const frac = Math.min(1, Math.max(0, (clientX - rect.left) / rect.width));
      onSeek(frac * span);
    },
    [onSeek, span],
  );

  if (cues.length === 0) return null;

  const playheadPct = span > 0 ? Math.min(100, Math.max(0, (currentTime / span) * 100)) : 0;

  return (
    <div className={cn('rounded-md bg-black/40 p-1', className)}>
      <div
        ref={wrapRef}
        role="slider"
        aria-label="Seek within recording"
        aria-valuemin={0}
        aria-valuemax={Math.round(span)}
        aria-valuenow={Math.round(Math.min(currentTime, span))}
        aria-valuetext={formatClock(currentTime)}
        tabIndex={0}
        className="relative w-full cursor-pointer touch-none select-none focus:outline-none focus-visible:ring-1 focus-visible:ring-fg/60"
        style={{ height: STRIP_HEIGHT }}
        onPointerDown={(e) => {
          draggingRef.current = true;
          e.currentTarget.setPointerCapture(e.pointerId);
          seekFromPointer(e.clientX);
        }}
        onPointerMove={(e) => {
          if (draggingRef.current) seekFromPointer(e.clientX);
        }}
        onPointerUp={() => {
          draggingRef.current = false;
        }}
        onKeyDown={(e) => {
          if (e.key === 'ArrowRight') onSeek(Math.min(span, currentTime + interval));
          if (e.key === 'ArrowLeft') onSeek(Math.max(0, currentTime - interval));
        }}
      >
        <canvas ref={canvasRef} className="h-full w-full" />
        <div
          aria-hidden
          className="pointer-events-none absolute inset-y-0"
          style={{ left: `${playheadPct}%` }}
        >
          <div className="h-full w-0.5 bg-danger" />
          <span className="absolute left-1 top-0 rounded bg-black/80 px-1 font-mono text-[9px] text-white">
            {formatClock(currentTime)}
          </span>
        </div>
      </div>
    </div>
  );
}
