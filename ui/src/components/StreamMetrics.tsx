import { useEffect, useState } from 'react';
import { useStreamStore } from '../hooks/useStreamStore';

interface StreamMetricsProps {
  streamId: string;
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

export function StreamMetrics({ streamId }: Readonly<StreamMetricsProps>) {
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

  return (
    <>
      <div className="flex justify-between">
        <span className="text-gray-600 dark:text-gray-300">Uptime:</span>
        <span className="text-gray-900 dark:text-white font-medium font-mono">
          {uptime}
        </span>
      </div>

      {metrics?.fps && (
        <div className="flex justify-between">
          <span className="text-gray-600 dark:text-gray-300">FPS:</span>
          <span className="text-gray-900 dark:text-white font-medium font-mono">
            {metrics.fps}
          </span>
        </div>
      )}

      {hasDroppedOrDuplicate && (
        <div className="flex justify-between">
          <span className="text-gray-600 dark:text-gray-300">Dropped / Duplicate:</span>
          <span className="text-gray-900 dark:text-white font-medium font-mono">
            {metrics?.dropped_frames ?? '0'} / {metrics?.duplicate_frames ?? '0'}
          </span>
        </div>
      )}
    </>
  );
}
