import { useState, useCallback, useRef, useEffect } from 'react';
import { ArrowPathIcon } from '@heroicons/react/24/outline';
import { api, API_BASE_URL } from '../../lib/api';
import { ICON_SIZE } from '../../utils';

export type Corner = [number, number];

const CORNER_LABELS = ['TL', 'TR', 'BR', 'BL'] as const;
const DOT_RADIUS = 10;
const DOT_HIT_RADIUS = 18;

const SOURCE_SNAPSHOT_ENDPOINT = '/api/sources/{source_id}/snapshot' as const;

function sortCornersClockwise(points: Corner[]): Corner[] {
  if (points.length !== 4) return points;
  const cx = points.reduce((s, p) => s + p[0], 0) / 4;
  const cy = points.reduce((s, p) => s + p[1], 0) / 4;
  const ordered = [...points].sort(
    (a, b) => Math.atan2(a[1] - cy, a[0] - cx) - Math.atan2(b[1] - cy, b[0] - cx),
  );
  let minIdx = 0;
  let minSum = Infinity;
  for (const [i, pt] of ordered.entries()) {
    if (pt) {
      const sum = pt[0] + pt[1];
      if (sum < minSum) {
        minSum = sum;
        minIdx = i;
      }
    }
  }
  return [...ordered.slice(minIdx), ...ordered.slice(0, minIdx)];
}

interface PerspectiveCanvasProps {
  /** Snapshot source: a source id whose raw NV12 snapshot endpoint provides the live preview backdrop. */
  snapshotSourceId: string | null;
  corners: Corner[];
  onCornersChange: (corners: Corner[], sorted: boolean) => void;
  sorted: boolean;
  inputWidth: number;
  inputHeight: number;
}

export function PerspectiveCanvas({
  snapshotSourceId,
  corners,
  onCornersChange,
  sorted,
  inputWidth,
  inputHeight,
}: Readonly<PerspectiveCanvasProps>) {
  const [snapshotUrl, setSnapshotUrl] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [imageLoaded, setImageLoaded] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [draggingIndex, setDraggingIndex] = useState<number | null>(null);
  const imgRef = useRef<HTMLImageElement>(null);

  const takeSnapshot = useCallback(async () => {
    if (!snapshotSourceId) {
      setError('No preview source available');
      return;
    }
    setLoading(true);
    setError(null);
    setImageLoaded(false);
    try {
      const { data, error: snapErr } = await api.POST(SOURCE_SNAPSHOT_ENDPOINT, {
        params: { path: { source_id: snapshotSourceId } },
      });
      if (snapErr) throw new Error(snapErr.detail ?? 'Snapshot failed');
      if (data?.url) setSnapshotUrl(`${API_BASE_URL}${data.url}`);
    } catch (error_) {
      setError('Failed to capture snapshot');
      console.error(error_);
    } finally {
      setLoading(false);
    }
  }, [snapshotSourceId]);

  useEffect(() => {
    if (snapshotSourceId) takeSnapshot();
  }, [snapshotSourceId, takeSnapshot]);

  const getImageCoords = useCallback(
    (clientX: number, clientY: number): Corner | null => {
      const img = imgRef.current;
      if (!img) return null;
      const rect = img.getBoundingClientRect();
      return [
        Math.round((clientX - rect.left) * (inputWidth / rect.width)),
        Math.round((clientY - rect.top) * (inputHeight / rect.height)),
      ];
    },
    [inputWidth, inputHeight],
  );

  const handleImageClick = useCallback(
    (e: React.MouseEvent<HTMLImageElement>) => {
      if (draggingIndex !== null || corners.length >= 4) return;
      const coord = getImageCoords(e.clientX, e.clientY);
      if (!coord) return;
      const next = [...corners, coord];
      if (next.length === 4) {
        onCornersChange(sortCornersClockwise(next), true);
      } else {
        onCornersChange(next, false);
      }
    },
    [draggingIndex, corners, getImageCoords, onCornersChange],
  );

  const handleDotMouseDown = useCallback((e: React.MouseEvent, index: number) => {
    e.preventDefault();
    e.stopPropagation();
    setDraggingIndex(index);
  }, []);

  const handleMouseMove = useCallback(
    (e: React.MouseEvent) => {
      if (draggingIndex === null) return;
      const coord = getImageCoords(e.clientX, e.clientY);
      if (!coord) return;
      const updated = [...corners] as Corner[];
      updated[draggingIndex] = coord;
      onCornersChange(updated, sorted);
    },
    [draggingIndex, corners, sorted, getImageCoords, onCornersChange],
  );

  const handleMouseUp = useCallback(() => {
    setDraggingIndex(null);
  }, []);

  const renderOverlay = () => {
    if (!imgRef.current || !imageLoaded || corners.length === 0) return null;
    const img = imgRef.current;
    if (img.naturalWidth === 0 || img.naturalHeight === 0) return null;
    const rect = img.getBoundingClientRect();
    const sx = rect.width / inputWidth;
    const sy = rect.height / inputHeight;
    const scaled = corners.map(([x, y]) => [x * sx, y * sy]);

    return (
      <svg
        className="absolute top-0 left-0 z-10"
        width={rect.width}
        height={rect.height}
        style={{ pointerEvents: 'none' }}
      >
        {scaled.length >= 2 && (
          <polygon
            points={scaled.map(([x, y]) => `${x},${y}`).join(' ')}
            fill="rgba(59, 130, 246, 0.15)"
            stroke="#3b82f6"
            strokeWidth="2"
            strokeDasharray={scaled.length < 4 ? '6 3' : undefined}
          />
        )}
        {scaled.map(([x, y], i) => (
          <g
            key={i}
            style={{ pointerEvents: 'all', cursor: draggingIndex === i ? 'grabbing' : 'grab' }}
            onMouseDown={(e) => handleDotMouseDown(e, i)}
          >
            <circle cx={x} cy={y} r={DOT_HIT_RADIUS} fill="transparent" />
            <circle cx={x} cy={y} r={DOT_RADIUS} fill="#3b82f6" stroke="white" strokeWidth="2" />
            <text
              x={x}
              y={Number(y) + 1}
              textAnchor="middle"
              dominantBaseline="central"
              fill="white"
              fontSize="10"
              fontWeight="bold"
            >
              {sorted ? CORNER_LABELS[i] : i + 1}
            </text>
          </g>
        ))}
      </svg>
    );
  };

  return (
    <>
      <div
        className="relative bg-surface-muted rounded-lg overflow-hidden mb-4"
        onMouseMove={handleMouseMove}
        onMouseUp={handleMouseUp}
        onMouseLeave={handleMouseUp}
      >
        {loading && (
          <div className="flex items-center justify-center h-64">
            <div className="text-fg-subtle">Capturing snapshot...</div>
          </div>
        )}
        {snapshotUrl && !loading && (
          <div className="relative inline-block w-full">
            <img
              ref={imgRef}
              src={snapshotUrl}
              alt="Source snapshot"
              className="w-full select-none cursor-crosshair"
              onClick={handleImageClick}
              onLoad={() => setImageLoaded(true)}
              draggable={false}
            />
            {renderOverlay()}
            {snapshotSourceId && (
              <button
                onClick={takeSnapshot}
                aria-label="Retake snapshot"
                className="absolute top-2 right-2 z-20 p-1.5 bg-surface-overlay hover:bg-surface-overlay rounded text-fg-inverse transition focus-visible:ring-2 focus-visible:ring-focus-ring"
                title="Retake snapshot"
              >
                <ArrowPathIcon className={ICON_SIZE.SM} />
              </button>
            )}
          </div>
        )}
        {!loading && !snapshotUrl && !error && (
          <div className="flex items-center justify-center h-64">
            <div className="text-fg-subtle">No snapshot available</div>
          </div>
        )}
        {error && !loading && (
          <div className="flex items-center justify-center h-64">
            <div className="text-danger">{error}</div>
          </div>
        )}
      </div>

      {corners.length > 0 && (
        <div className="flex flex-wrap gap-3 mb-4 text-sm font-mono text-fg-muted">
          {corners.map(([x, y], i) => (
            <span key={i} className="px-2 py-1 bg-surface-muted rounded">
              {sorted ? CORNER_LABELS[i] : `${i + 1}`}({x},{y})
            </span>
          ))}
        </div>
      )}
    </>
  );
}
