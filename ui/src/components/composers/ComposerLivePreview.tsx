import { useState } from 'react';
import { Card } from '../Card';
import { Button } from '../Button';
import { SectionHeader } from '../primitives/SectionHeader';
import { LivePreviewFrame } from '../primitives/LivePreviewFrame';
import { API_BASE_URL } from '../../lib/api';
import { useStreamStore } from '../../hooks/useStreamStore';

interface ComposerLivePreviewProps {
  composerId: string;
  /**
   * Initial fps for the daemon's preview.mjpg stream. Clamped server-side
   * to [1, server.PreviewMaxFPS]. Default 1.
   */
  initialFps?: number;
}

export function ComposerLivePreview({
  composerId,
  initialFps = 1,
}: Readonly<ComposerLivePreviewProps>) {
  const pipelineEnabled = useStreamStore((s) => s.pipelineEnabled);
  const [fps] = useState(initialFps);
  const [streaming, setStreaming] = useState(true);

  const pipelineOff = pipelineEnabled === false;
  const pipelineUnknown = pipelineEnabled === null;

  const src = streaming && !pipelineOff && !pipelineUnknown
    ? `${API_BASE_URL}/api/composers/${encodeURIComponent(composerId)}/preview.mjpg?fps=${fps}`
    : undefined;

  return (
    <Card padding="lg">
      <SectionHeader
        title="Live preview"
        description={pipelineOff ? 'Pipeline stopped.' : `Composer canvas streaming at ${fps.toFixed(1)} Hz.`}
        actions={
          !pipelineOff && !pipelineUnknown ? (
            <Button
              text={streaming ? 'Pause' : 'Resume'}
              theme="light"
              size="SM"
              onClick={() => setStreaming((v) => !v)}
            />
          ) : undefined
        }
      />
      <LivePreviewFrame
        {...(src !== undefined ? { src } : {})}
        {...(pipelineUnknown && { state: 'loading' as const })}
        {...(pipelineOff && { state: 'idle' as const })}
        idleMessage="Pipeline stopped"
        loading={false}
        error={null}
        alt={`Live preview of composer ${composerId}`}
      />
    </Card>
  );
}
