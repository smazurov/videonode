import { Link } from 'react-router-dom';
import { useStreamStore } from '../../hooks/useStreamStore';
import { LivePreviewFrame } from '../primitives/LivePreviewFrame';
import { StatusPill } from '../primitives/StatusPill';
import { SectionHeader } from '../primitives/SectionHeader';
import { Badge } from '../Badge';
import { resolveUpstream } from './upstream';
import { cn } from '../../utils';

interface StreamOverviewPanelProps {
  readonly streamId: string;
  readonly className?: string;
}

export function StreamOverviewPanel({ streamId, className }: StreamOverviewPanelProps) {
  const stream = useStreamStore((state) => state.streamsById[streamId]);
  const refreshKey = useStreamStore((state) => state.streamRefreshKeys[streamId] ?? 0);

  if (!stream) {
    return (
      <section className={cn('rounded-lg border border-border bg-surface-raised p-4', className)}>
        <p className="text-sm text-fg-muted">Stream not found.</p>
      </section>
    );
  }

  const upstream = resolveUpstream(stream);
  const enabled = !!stream.enabled;
  const sourceStreams = stream.canvas?.source_streams ?? [];
  const readerCount = sourceStreams.length;

  return (
    <section className={cn('space-y-4 rounded-lg border border-border bg-surface-raised p-4', className)}>
      <SectionHeader
        title="Live preview"
        description="WebRTC playback of this stream"
        actions={
          <StatusPill tone={enabled ? 'running' : 'idle'}>
            {enabled ? 'running' : 'idle'}
          </StatusPill>
        }
      />

      <LivePreviewFrame
        streamId={streamId}
        enabled={enabled}
        refreshKey={refreshKey}
        showStats={false}
      />

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
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
          <div className="text-xs uppercase tracking-wide text-fg-muted">Readers</div>
          <div className="mt-1 font-mono text-lg text-fg">{readerCount}</div>
        </div>
        <div className="rounded-md border border-border bg-surface-muted/30 p-3">
          <div className="text-xs uppercase tracking-wide text-fg-muted">Encoder</div>
          <div className="mt-1">
            <StatusPill tone={enabled ? 'running' : 'idle'}>
              {enabled ? 'running' : 'idle'}
            </StatusPill>
          </div>
        </div>
      </div>
    </section>
  );
}
