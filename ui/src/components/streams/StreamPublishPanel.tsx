import { useStreamStore } from '../../hooks/useStreamStore';
import { SectionHeader } from '../primitives/SectionHeader';
import { Badge, type BadgeProps } from '../Badge';
import { buildStreamURL } from '../../lib/api';
import { cn } from '../../utils';

interface StreamPublishPanelProps {
  readonly streamId: string;
  readonly className?: string;
}

interface PublishTarget {
  readonly type: string;
  readonly url: string;
}

type PublishTone = NonNullable<BadgeProps['tone']>;

const TYPE_TONE: Record<string, PublishTone> = {
  rtsp: 'rtsp',
  srt: 'srt',
  rtmp: 'rtmp',
  hls: 'info',
  webrtc: 'webrtc',
};

function derivePublishTargets(stream: { rtsp_url?: string; srt_url?: string; publish?: unknown }): PublishTarget[] {
  const explicit = Array.isArray(stream.publish) ? (stream.publish as PublishTarget[]) : [];
  if (explicit.length > 0) return explicit;
  // Fall back to the legacy implicit publish set: each stream auto-publishes
  // to the bundled RTSP / SRT relays.
  const out: PublishTarget[] = [];
  if (stream.rtsp_url) {
    out.push({ type: 'rtsp', url: buildStreamURL(stream.rtsp_url, 'rtsp') ?? stream.rtsp_url });
  }
  if (stream.srt_url) {
    out.push({ type: 'srt', url: buildStreamURL(stream.srt_url, 'srt') ?? stream.srt_url });
  }
  return out;
}

export function StreamPublishPanel({ streamId, className }: StreamPublishPanelProps) {
  const stream = useStreamStore((state) => state.streamsById[streamId]);

  if (!stream) {
    return (
      <section className={cn('rounded-lg border border-border bg-surface-raised p-4', className)}>
        <p className="text-sm text-fg-muted">Stream not found.</p>
      </section>
    );
  }

  const targets = derivePublishTargets(stream);

  return (
    <section className={cn('rounded-lg border border-border bg-surface-raised p-4', className)}>
      <SectionHeader title="Publish targets" description="Where this stream is broadcast" />
      <div className="mt-3 space-y-2">
        {targets.length === 0 ? (
          <div className="rounded-md border border-dashed border-border px-4 py-6 text-center text-xs text-fg-muted">
            No publish targets configured.
          </div>
        ) : (
          targets.map((target, idx) => {
            const tone = TYPE_TONE[target.type.toLowerCase()] ?? 'neutral';
            return (
              <div
                key={`${target.type}-${idx}`}
                className="flex items-center gap-3 rounded-md border border-border bg-surface-muted/30 px-3 py-2"
              >
                <Badge tone={tone} size="sm">{target.type.toUpperCase()}</Badge>
                <code className="min-w-0 flex-1 truncate font-mono text-xs text-fg" title={target.url}>
                  {target.url}
                </code>
              </div>
            );
          })
        )}
      </div>
    </section>
  );
}
