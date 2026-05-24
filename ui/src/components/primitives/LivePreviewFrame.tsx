import type { ReactNode } from 'react';
import { WebRTCPlayer } from '../webrtc';
import { cn } from '../../utils';

interface LivePreviewFrameProps {
  readonly streamId: string;
  readonly enabled?: boolean;
  readonly showStats?: boolean;
  readonly className?: string;
  readonly refreshKey?: number;
  readonly overlay?: ReactNode;
  readonly placeholder?: ReactNode;
}

export function LivePreviewFrame({
  streamId,
  enabled = true,
  showStats = false,
  className,
  refreshKey,
  overlay,
  placeholder,
}: LivePreviewFrameProps) {
  return (
    <div className={cn('relative aspect-video overflow-hidden rounded-lg bg-surface-muted', className)}>
      {enabled ? (
        <WebRTCPlayer
          key={refreshKey ?? 0}
          streamId={streamId}
          className="h-full w-full"
          showStats={showStats}
        />
      ) : (
        <div className="flex h-full w-full items-center justify-center text-sm text-fg-muted">
          {placeholder ?? 'Stream disabled'}
        </div>
      )}
      {overlay && <div className="pointer-events-none absolute inset-0">{overlay}</div>}
    </div>
  );
}
