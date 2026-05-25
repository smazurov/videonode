import { useState } from 'react';
import { Card } from '../Card';
import { Button } from '../Button';
import { SectionHeader } from '../primitives/SectionHeader';
import { LivePreviewFrame } from '../primitives/LivePreviewFrame';
import { API_BASE_URL } from '../../lib/api';

interface ComposerLivePreviewProps {
  composerId: string;
  visible: boolean;
  onToggle: () => void;
  initialFps?: number;
}

export function ComposerLivePreview({
  composerId,
  visible,
  onToggle,
  initialFps = 1,
}: Readonly<ComposerLivePreviewProps>) {
  const [fps] = useState(initialFps);

  if (!visible) return null;

  const src = `${API_BASE_URL}/api/composers/${encodeURIComponent(composerId)}/preview.mjpg?fps=${fps}`;

  return (
    <Card padding="lg">
      <SectionHeader
        title="Live preview"
        description={`Composer canvas streaming at ${fps.toFixed(1)} Hz.`}
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
        alt={`Live preview of composer ${composerId}`}
      />
    </Card>
  );
}
