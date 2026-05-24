import { useEffect, useState } from 'react';
import { useStreamStore } from '../hooks/useStreamStore';

interface StreamMetricsProps {
  readonly streamId: string;
  readonly layout?: 'inline' | 'stacked';
}

function calculateUptime(startTime?: string): string {
  if (!startTime) return 'N/A';
  try {
    const start = new Date(startTime);
    const now = new Date();
    const uptimeMs = now.getTime() - start.getTime();
    if (uptimeMs < 0) return 'N/A';
    const seconds = Math.floor(uptimeMs / 1000);
    const days = Math.floor(seconds / 86400);
    const hours = Math.floor((seconds % 86400) / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    const remainingSeconds = seconds % 60;
    if (days > 0) {
      return `${days}d, ${hours}h, ${minutes}m, ${remainingSeconds}s`;
    }
    return `${hours}h, ${minutes}m, ${remainingSeconds}s`;
  } catch {
    return 'N/A';
  }
}

// Compact metrics readout used inline in stream lists / overview panels.
// Detailed history view lives in `streams/StreamMetricsPanel.tsx`.
export function StreamMetrics({ streamId, layout = 'stacked' }: Readonly<StreamMetricsProps>) {
  const metrics = useStreamStore((state) => state.metricsById[streamId]);
  const startTime = useStreamStore((state) => state.streamsById[streamId]?.start_time);
  const [uptime, setUptime] = useState(() => calculateUptime(startTime));

  useEffect(() => {
    const interval = setInterval(() => {
      setUptime(calculateUptime(startTime));
    }, 1000);
    return () => clearInterval(interval);
  }, [startTime]);

  const hasDroppedOrDuplicate = metrics?.dropped_frames || metrics?.duplicate_frames;

  if (layout === 'inline') {
    return (
      <span className="inline-flex items-center gap-3 font-mono text-xs text-fg-muted">
        <span>{uptime}</span>
        {metrics?.fps && <span>{metrics.fps} fps</span>}
        {hasDroppedOrDuplicate && (
          <span>
            {metrics?.dropped_frames ?? '0'}/{metrics?.duplicate_frames ?? '0'} d/d
          </span>
        )}
      </span>
    );
  }

  return (
    <>
      <div className="flex justify-between">
        <span className="text-fg-muted">Uptime:</span>
        <span className="font-mono font-medium text-fg">{uptime}</span>
      </div>

      {metrics?.fps && (
        <div className="flex justify-between">
          <span className="text-fg-muted">FPS:</span>
          <span className="font-mono font-medium text-fg">{metrics.fps}</span>
        </div>
      )}

      {hasDroppedOrDuplicate && (
        <div className="flex justify-between">
          <span className="text-fg-muted">Dropped / Duplicate:</span>
          <span className="font-mono font-medium text-fg">
            {metrics?.dropped_frames ?? '0'} / {metrics?.duplicate_frames ?? '0'}
          </span>
        </div>
      )}
    </>
  );
}
