import { Link } from 'react-router-dom';
import { useStreamStore } from '../../hooks/useStreamStore';
import { LivePreviewFrame } from '../primitives/LivePreviewFrame';
import { StatusPill } from '../primitives/StatusPill';
import { poolStateToPill } from '../../lib/pool-status';
import { SectionHeader } from '../primitives/SectionHeader';
import { Badge } from '../Badge';
import { WebRTCPlayer } from '../webrtc';
import { resolveUpstream } from './upstream';
import { cn } from '../../utils';

interface StreamOverviewPanelProps {
  readonly streamId: string;
  readonly className?: string;
  /** When true, the large WebRTC preview is not rendered. */
  readonly videoHidden?: boolean;
}

export function StreamOverviewPanel({ streamId, className, videoHidden = false }: StreamOverviewPanelProps) {
  const stream = useStreamStore((state) => state.streamsById[streamId]);
  const refreshKey = useStreamStore((state) => state.streamRefreshKeys[streamId] ?? 0);
  const pipelineEnabled = useStreamStore((state) => state.pipelineEnabled);

  if (!stream) {
    return (
      <section className={cn('rounded-lg border border-border bg-surface-raised p-4', className)}>
        <p className="text-sm text-fg-muted">Stream not found.</p>
      </section>
    );
  }

  const upstream = resolveUpstream(stream);
  const status = poolStateToPill(stream.status);
  // Mount the player whenever the pipeline is on, even if the stream is idle:
  // the offer it sends is what lazily wakes the encoder (lazy-encoder-on-reader).
  const canPreview = pipelineEnabled !== false;

  return (
    <section className={cn('space-y-4 rounded-lg border border-border bg-surface-raised p-4', className)}>
      <SectionHeader
        title="Live preview"
        description="WebRTC playback of this stream"
        actions={<StatusPill status={status} />}
      />

      {!videoHidden && (
        <LivePreviewFrame
          state={canPreview ? 'ready' : 'idle'}
          idleMessage="Pipeline stopped"
          mediaClassName="bg-black"
        >
          {canPreview && (
            <WebRTCPlayer
              key={`${streamId}:${refreshKey}`}
              streamId={streamId}
              className="w-full h-full"
              muted
              showStats
            />
          )}
        </LivePreviewFrame>
      )}

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <div className="rounded-md border border-border bg-surface-muted/30 p-3">
          <div className="text-xs uppercase tracking-wide text-fg-muted">Upstream</div>
          <div className="mt-1">
            {upstream.href ? (
              <Link to={upstream.href} className="inline-flex hover:opacity-80">
                <Badge tone={upstream.kind === 'composer' ? 'canvas' : 'info'} size="md">
                  {upstream.raw}
                </Badge>
              </Link>
            ) : (
              <Badge tone="neutral" size="md">{upstream.raw}</Badge>
            )}
          </div>
        </div>
        <div className="rounded-md border border-border bg-surface-muted/30 p-3">
          <div className="text-xs uppercase tracking-wide text-fg-muted">Encoder</div>
          <div className="mt-1">
            <StatusPill status={status} />
          </div>
        </div>
      </div>
    </section>
  );
}
