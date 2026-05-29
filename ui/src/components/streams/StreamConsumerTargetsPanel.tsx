import { Link } from 'react-router-dom';
import { useStreamStore } from '../../hooks/useStreamStore';
import { SectionHeader } from '../primitives/SectionHeader';
import { Badge, type BadgeProps } from '../Badge';
import { buildStreamURL } from '../../lib/api';
import { cn } from '../../utils';

interface StreamConsumerTargetsPanelProps {
  readonly streamId: string;
  readonly className?: string;
}

type Tone = NonNullable<BadgeProps['tone']>;

function TargetRow({ tone, label, children }: { readonly tone: Tone; readonly label: string; readonly children: React.ReactNode }) {
  return (
    <div className="flex items-center gap-3 rounded-md border border-border bg-surface-muted/30 px-3 py-2">
      <Badge tone={tone} size="sm">{label}</Badge>
      {children}
    </div>
  );
}

function URLValue({ url }: { readonly url: string | undefined }) {
  if (!url) return <span className="min-w-0 flex-1 text-xs text-fg-subtle">—</span>;
  return (
    <code className="min-w-0 flex-1 truncate font-mono text-xs text-fg" title={url}>
      {url}
    </code>
  );
}

// StreamConsumerPanel lists the endpoints viewers connect to read this
// stream — RTSP, SRT, and the WebRTC player. Mirrors the columns on the
// stream list. The encoder's publish target (local RTSP relay) is an
// internal detail and is intentionally not surfaced here.
export function StreamConsumerTargetsPanel({ streamId, className }: StreamConsumerTargetsPanelProps) {
  const stream = useStreamStore((state) => state.streamsById[streamId]);

  if (!stream) {
    return (
      <section className={cn('rounded-lg border border-border bg-surface-raised p-4', className)}>
        <p className="text-sm text-fg-muted">Stream not found.</p>
      </section>
    );
  }

  const rtsp = stream.rtsp_url ? buildStreamURL(stream.rtsp_url, 'rtsp') ?? stream.rtsp_url : undefined;
  const srt = stream.srt_url ? buildStreamURL(stream.srt_url, 'srt') ?? stream.srt_url : undefined;

  return (
    <section className={cn('rounded-lg border border-border bg-surface-raised p-4', className)}>
      <SectionHeader title="Consumer targets" description="Where viewers connect to watch this stream" />
      <div className="mt-3 space-y-2">
        <TargetRow tone="rtsp" label="RTSP">
          <URLValue url={rtsp} />
        </TargetRow>
        <TargetRow tone="srt" label="SRT">
          <URLValue url={srt} />
        </TargetRow>
        <TargetRow tone="webrtc" label="WebRTC">
          <Link
            to={`/video?stream=${encodeURIComponent(streamId)}`}
            className="text-xs text-fg-muted underline-offset-2 hover:underline"
          >
            open in player
          </Link>
        </TargetRow>
      </div>
    </section>
  );
}
