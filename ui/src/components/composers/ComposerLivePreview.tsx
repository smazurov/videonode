import { useState } from 'react';
import { Card } from '../Card';
import { Button } from '../Button';
import { SectionHeader } from '../primitives/SectionHeader';
import { LivePreviewFrame } from '../primitives/LivePreviewFrame';
import { API_BASE_URL } from '../../lib/api';

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
  const [fps] = useState(initialFps);
  const [streaming, setStreaming] = useState(true);

  const src = streaming
    ? `${API_BASE_URL}/api/composers/${encodeURIComponent(composerId)}/preview.mjpg?fps=${fps}`
    : undefined;

  return (
    <Card padding="lg">
      <SectionHeader
        title="Live preview"
        description={`Composer canvas streaming at ${fps.toFixed(1)} Hz.`}
        actions={
          <Button
            text={streaming ? 'Pause' : 'Resume'}
            theme="light"
            size="SM"
            onClick={() => setStreaming((v) => !v)}
          />
        }
      />
      <LivePreviewFrame
        {...(src !== undefined ? { src } : {})}
        loading={false}
        error={null}
        alt={`Live preview of composer ${composerId}`}
      />
    </Card>
  );
}
