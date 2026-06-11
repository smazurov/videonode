import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import toast from 'react-hot-toast';
import { StopIcon } from '@heroicons/react/24/outline';
import { useStreamStore } from '../../hooks/useStreamStore';
import { useRecordingStore } from '../../hooks/useRecordingStore';
import { Button } from '../Button';
import { apiUrl } from '../../lib/api_fetch';
import { fetchStoryboard, SPRITE_GRID, type StoryboardCue } from '../../lib/storyboard';
import { formatClock } from './format';
import type { components } from '../../lib/api.generated';

type RecordingStatus = components['schemas']['RecordingStatusData'];

interface RecordingControlsProps {
  readonly streamId: string;
}

// RecordingControls is the compact recording cell on the stream overview
// panel: record/stop, a running timer, the latest storyboard frame, and a
// link to the recordings route for playback. State syncs over recording.*
// SSE events via useRecordingStore — no polling.
export function RecordingControls({ streamId }: RecordingControlsProps) {
  const startRecording = useStreamStore((s) => s.startRecording);
  const stopRecording = useStreamStore((s) => s.stopRecording);

  const recordingsById = useRecordingStore((s) => s.recordingsById);
  const storeLoaded = useRecordingStore((s) => s.loaded);
  const fetchRecordings = useRecordingStore((s) => s.fetchRecordings);
  const upsertRecording = useRecordingStore((s) => s.upsertRecording);

  // Active session for this stream, else its most recent one.
  const status: RecordingStatus | null = useMemo(() => {
    const mine = Object.values(recordingsById).filter((r) => r.stream_id === streamId);
    if (mine.length === 0) return null;
    const newestFirst = [...mine].sort((a, b) =>
      (b.started_at ?? '').localeCompare(a.started_at ?? ''),
    );
    return mine.find((r) => r.active) ?? newestFirst[0] ?? null;
  }, [recordingsById, streamId]);

  const [busy, setBusy] = useState(false);
  const [latest, setLatest] = useState<{ cue: StoryboardCue; total: number } | null>(null);
  const [nowMs, setNowMs] = useState(() => Date.now());

  const active = status?.active ?? false;
  const vttUrl = apiUrl(status?.thumbnails_vtt_url);
  const sessionBase = vttUrl.replace(/\/thumbnails\.vtt$/, '');
  const detailHref = status
    ? `/recordings/${encodeURIComponent(status.stream_id)}/${encodeURIComponent(status.recording_id)}`
    : '';
  const elapsedSec = active
    ? Math.max(0, (nowMs - Date.parse(status?.started_at ?? '')) / 1000)
    : (status?.duration_seconds ?? 0);

  useEffect(() => {
    if (!storeLoaded) void fetchRecordings();
  }, [storeLoaded, fetchRecordings]);

  // Ticks only while recording; a stopped session displays the server's
  // duration_seconds instead.
  useEffect(() => {
    if (!active) return;
    const timer = window.setInterval(() => setNowMs(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [active]);

  useEffect(() => {
    if (!vttUrl) return;
    let cancelled = false;
    const load = async () => {
      const cues = await fetchStoryboard(vttUrl);
      const last = cues?.[cues.length - 1];
      if (!cancelled && cues && last) setLatest({ cue: last, total: cues.length });
    };
    void load();
    if (!active) return;
    const timer = window.setInterval(() => void load(), 5000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [vttUrl, active]);

  const handleStart = useCallback(async () => {
    setBusy(true);
    try {
      setLatest(null);
      setNowMs(Date.now());
      upsertRecording(await startRecording(streamId));
      toast.success('Recording started');
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Failed to start recording');
    } finally {
      setBusy(false);
    }
  }, [streamId, startRecording, upsertRecording]);

  const handleStop = useCallback(async () => {
    setBusy(true);
    try {
      upsertRecording(await stopRecording(streamId));
      toast.success('Recording stopped');
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Failed to stop recording');
    } finally {
      setBusy(false);
    }
  }, [streamId, stopRecording, upsertRecording]);

  return (
    <div className="rounded-md border border-border bg-surface-muted/30 p-3">
      <div className="flex items-center justify-between gap-2">
        <div className="text-xs uppercase tracking-wide text-fg-muted">Recording</div>
        {active && (
          <span className="flex items-center gap-1.5 text-xs font-medium text-danger">
            <span className="h-2 w-2 animate-pulse rounded-full bg-danger" />
            REC
          </span>
        )}
      </div>
      <div className="mt-1 flex items-center justify-between gap-3">
        {status ? (
          <div className="flex min-w-0 items-center gap-2">
            <Link
              to={detailHref}
              className="shrink-0 overflow-hidden rounded border border-border hover:border-fg/60"
            >
              {latest ? (
                <SpriteThumb
                  cue={latest.cue}
                  src={
                    `${sessionBase}/${latest.cue.url}` + (active ? `?v=${latest.total}` : '')
                  }
                  heightPx={40}
                />
              ) : (
                <span className="flex h-10 w-[71px] items-center justify-center bg-black/40 text-[9px] text-fg-muted">
                  no frame
                </span>
              )}
            </Link>
            <div className="flex min-w-0 flex-col">
              <span className="font-mono text-base font-medium tabular-nums leading-tight text-fg">
                {formatClock(elapsedSec)}
              </span>
              <Link to={detailHref} className="truncate text-xs text-accent hover:underline">
                {active ? 'Watch →' : 'View →'}
              </Link>
            </div>
          </div>
        ) : (
          <span className="text-sm text-fg-muted">Not recording</span>
        )}
        {active ? (
          <Button
            theme="danger"
            size="SM"
            onClick={handleStop}
            disabled={busy}
            LeadingIcon={StopIcon}
            text="Stop"
          />
        ) : (
          <Button
            theme="light"
            size="SM"
            onClick={handleStart}
            disabled={busy}
            LeadingIcon={RecordDot}
            text="Record"
          />
        )}
      </div>
    </div>
  );
}

// SpriteThumb renders one storyboard frame: a CSS background crop out of the
// sprite sheet for #xywh cues, or a plain <img> for legacy per-frame cues.
function SpriteThumb({
  cue,
  src,
  heightPx,
}: {
  readonly cue: StoryboardCue;
  readonly src: string;
  readonly heightPx: number;
}) {
  const alt = `latest frame at ${formatClock(cue.start)}`;
  if (!cue.xywh) {
    return <img src={src} alt={alt} style={{ height: heightPx }} className="w-auto" />;
  }
  const scale = heightPx / cue.xywh.h;
  return (
    <div
      role="img"
      aria-label={alt}
      style={{
        width: Math.round(cue.xywh.w * scale),
        height: heightPx,
        backgroundImage: `url(${src})`,
        backgroundPosition: `-${cue.xywh.x * scale}px -${cue.xywh.y * scale}px`,
        backgroundSize: `${SPRITE_GRID * cue.xywh.w * scale}px ${SPRITE_GRID * cue.xywh.h * scale}px`,
      }}
    />
  );
}

// RecordDot is a small filled record glyph for the Record button.
function RecordDot(props: Readonly<React.SVGProps<SVGSVGElement>>) {
  return (
    <svg viewBox="0 0 20 20" fill="currentColor" aria-hidden="true" {...props}>
      <circle cx="10" cy="10" r="6" />
    </svg>
  );
}
