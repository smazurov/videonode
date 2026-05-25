import { useState } from 'react';
import { Card } from '../Card';
import { Button } from '../Button';
import { SectionHeader } from '../primitives/SectionHeader';
import { LivePreviewFrame } from '../primitives/LivePreviewFrame';
import { API_BASE_URL } from '../../lib/api';
import { useStreamStore } from '../../hooks/useStreamStore';

interface SourceLivePreviewProps {
  sourceId: string;
  visible: boolean;
  onToggle: () => void;
  initialFps?: number;
}

export function SourceLivePreview({
  sourceId,
  visible,
  onToggle,
  initialFps = 1,
}: Readonly<SourceLivePreviewProps>) {
  const pipelineEnabled = useStreamStore((s) => s.pipelineEnabled);
  const [fps] = useState(initialFps);

  if (!visible) return null;

  const pipelineOff = pipelineEnabled === false;
  const pipelineUnknown = pipelineEnabled === null;

  const src = !pipelineOff && !pipelineUnknown
    ? `${API_BASE_URL}/api/sources/${encodeURIComponent(sourceId)}/preview.mjpg?fps=${fps}`
    : undefined;

  return (
    <Card padding="lg">
      <SectionHeader
        title="Live preview"
        description={pipelineOff ? 'Pipeline stopped.' : `Streaming at ${fps.toFixed(1)} Hz.`}
        actions={
          <Button
            text="Hide"
            theme="light"
            size="SM"
            onClick={onToggle}
          />
        }
      />
      <LivePreviewFrame
        {...(src !== undefined ? { src } : {})}
        {...(pipelineUnknown && { state: 'loading' as const })}
        {...(pipelineOff && { state: 'idle' as const })}
        idleMessage="Pipeline stopped"
        loading={false}
        error={null}
        alt={`Live preview of source ${sourceId}`}
      />
    </Card>
  );
}
