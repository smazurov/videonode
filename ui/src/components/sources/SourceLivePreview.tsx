import { useState } from 'react';
import { Card } from '../Card';
import { Button } from '../Button';
import { SectionHeader } from '../primitives/SectionHeader';
import { LivePreviewFrame } from '../primitives/LivePreviewFrame';
import { API_BASE_URL } from '../../lib/api';

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
  const [fps] = useState(initialFps);

  if (!visible) return null;

  const src = `${API_BASE_URL}/api/sources/${encodeURIComponent(sourceId)}/preview.mjpg?fps=${fps}`;

  return (
    <Card padding="lg">
      <SectionHeader
        title="Live preview"
        description={`Streaming at ${fps.toFixed(1)} Hz.`}
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
        src={src}
        loading={false}
        error={null}
        alt={`Live preview of source ${sourceId}`}
      />
    </Card>
  );
}
