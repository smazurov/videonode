import { Card } from '../Card';
import { Button } from '../Button';
import { SectionHeader } from '../primitives/SectionHeader';
import { LivePreviewFrame } from '../primitives/LivePreviewFrame';
import { WebRTCPlayer } from '../webrtc';

interface StreamPreviewPanelProps {
  readonly streamId: string;
  /** False while the pipeline is stopped; the player is not mounted then. */
  readonly canPreview: boolean;
  readonly refreshKey: number;
  readonly onHide: () => void;
  /** Overlay WebRTC stats on the video (large preview). */
  readonly showStats?: boolean;
}

/**
 * WebRTC preview as viewers see it. Mounting the player is what lazily wakes
 * the encoder, so it renders whenever the pipeline is on.
 */
export function StreamPreviewPanel({ streamId, canPreview, refreshKey, onHide, showStats = false }: StreamPreviewPanelProps) {
  return (
    <Card padding="lg" className="space-y-4">
      <SectionHeader
        title="Live preview"
        description={canPreview ? undefined : 'Pipeline stopped.'}
        actions={<Button text="Hide" theme="light" size="SM" onClick={onHide} />}
      />
      <LivePreviewFrame state={canPreview ? 'ready' : 'idle'} idleMessage="Pipeline stopped" mediaClassName="bg-black">
        {canPreview && (
          <WebRTCPlayer key={`${streamId}:${refreshKey}`} streamId={streamId} className="h-full w-full" muted showStats={showStats} />
        )}
      </LivePreviewFrame>
    </Card>
  );
}
