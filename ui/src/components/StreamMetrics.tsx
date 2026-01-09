import { useStreamStore } from '../hooks/useStreamStore';

interface StreamMetricsProps {
  streamId: string;
}

export function StreamMetrics({ streamId }: Readonly<StreamMetricsProps>) {
  const metrics = useStreamStore((state) => state.metricsById[streamId]);

  console.log('[StreamMetrics] render', streamId, metrics);

  if (!metrics) return null;

  const hasDroppedOrDuplicate = metrics.dropped_frames || metrics.duplicate_frames;

  return (
    <>
      {metrics.fps && (
        <div className="flex justify-between">
          <span className="text-gray-600 dark:text-gray-300">FPS:</span>
          <span className="text-gray-900 dark:text-white font-medium">
            {metrics.fps}
          </span>
        </div>
      )}

      {hasDroppedOrDuplicate && (
        <div className="flex justify-between">
          <span className="text-gray-600 dark:text-gray-300">Dropped / Duplicate:</span>
          <span className="text-gray-900 dark:text-white font-medium">
            {metrics.dropped_frames ?? '0'} / {metrics.duplicate_frames ?? '0'}
          </span>
        </div>
      )}
    </>
  );
}
