import { useState, useCallback, useRef } from 'react';
import { ArrowPathIcon } from '@heroicons/react/24/outline';
import { API_BASE_URL } from '../../lib/api';
import { ICON_SIZE } from '../../utils';
import { useStreamStore } from '../../hooks/useStreamStore';

export type Corner = [number, number];

const CORNER_LABELS = ['TL', 'TR', 'BR', 'BL'] as const;
const DOT_RADIUS = 10;
const DOT_HIT_RADIUS = 18;

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

export interface SnapshotDims {
  w: number;
  h: number;
}

interface PerspectiveCanvasProps {
  /** Snapshot source: a source id whose raw NV12 snapshot endpoint provides the live preview backdrop. */
  snapshotSourceId: string | null;
  corners: Corner[];
  onCornersChange: (corners: Corner[], sorted: boolean) => void;
  sorted: boolean;
  /** Notifies parent of the snapshot's natural pixel dimensions on load. */
  onSnapshotDimsChange?: (dims: SnapshotDims) => void;
}

// Snapshot state bundled so a single setState atomically resets all fields.
interface SnapshotState {
  // null tick = no snapshot requested; non-null = a fetch is in progress or done.
  tick: number | null;
  naturalDims: SnapshotDims | null;
  error: string | null;
  // The source/pipeline key this snapshot was taken for, used to detect changes
  // during render without needing a sync-setState effect.
  forSource: string | null;
  forPipeline: boolean;
}

export function PerspectiveCanvas({
  snapshotSourceId,
  corners,
  onCornersChange,
  sorted,
  onSnapshotDimsChange,
}: Readonly<PerspectiveCanvasProps>) {
  const pipelineEnabled = useStreamStore((s) => s.pipelineEnabled);
  const pipelineActive = pipelineEnabled === true;

  // Use a monotonic generation counter as cache-buster (avoids Date.now() in render).
  const [snapshot, setSnapshot] = useState<SnapshotState>(() => ({
    tick: snapshotSourceId && pipelineActive ? 1 : null,
    naturalDims: null,
    error: null,
    forSource: snapshotSourceId,
    forPipeline: pipelineActive,
  }));
  const [draggingIndex, setDraggingIndex] = useState<number | null>(null);
  const imgRef = useRef<HTMLImageElement>(null);

  // When the source id or pipeline active state changes, reset and request a
  // new snapshot during render (no effect needed). React re-renders immediately
  // without painting the intermediate state, avoiding cascading renders.
  let effectiveSnapshot = snapshot;
  if (snapshot.forSource !== snapshotSourceId || snapshot.forPipeline !== pipelineActive) {
    const nextTick = snapshotSourceId && pipelineActive ? (snapshot.tick ?? 0) + 1 : null;
    effectiveSnapshot = {
      tick: nextTick,
      naturalDims: null,
      error: null,
      forSource: snapshotSourceId,
      forPipeline: pipelineActive,
    };
    setSnapshot(effectiveSnapshot);
  }

  const snapshotUrl =
    effectiveSnapshot.tick !== null && snapshotSourceId
      ? `${API_BASE_URL}/api/sources/${encodeURIComponent(snapshotSourceId)}/snapshot.jpg?t=${effectiveSnapshot.tick}`
      : null;

  // loading is derived: URL is set, but the image hasn't loaded or errored yet.
  const loading = snapshotUrl !== null && effectiveSnapshot.naturalDims === null && effectiveSnapshot.error === null;

  const retakeSnapshot = useCallback(() => {
    if (!snapshotSourceId) return;
    setSnapshot((prev) => ({
      ...prev,
      tick: (prev.tick ?? 0) + 1,
      naturalDims: null,
      error: null,
    }));
  }, [snapshotSourceId]);

  const getImageCoords = useCallback(
    (clientX: number, clientY: number): Corner | null => {
      const img = imgRef.current;
      if (!effectiveSnapshot.naturalDims) return null;
      const rect = img?.getBoundingClientRect();
      if (!rect) return null;
      return [
        Math.round((clientX - rect.left) * (effectiveSnapshot.naturalDims.w / rect.width)),
        Math.round((clientY - rect.top) * (effectiveSnapshot.naturalDims.h / rect.height)),
      ];
    },
    [effectiveSnapshot.naturalDims],
  );

  const handleImageClick = useCallback(
    (e: React.MouseEvent<HTMLImageElement>) => {
      if (!pipelineActive || draggingIndex !== null || corners.length >= 4) return;
      const coord = getImageCoords(e.clientX, e.clientY);
      if (!coord) return;
      const next = [...corners, coord];
      if (next.length === 4) {
        onCornersChange(sortCornersClockwise(next), true);
      } else {
        onCornersChange(next, false);
      }
    },
    [pipelineActive, draggingIndex, corners, getImageCoords, onCornersChange],
  );

  const handleDotMouseDown = useCallback((e: React.MouseEvent, index: number) => {
    if (!pipelineActive) return;
    e.preventDefault();
    e.stopPropagation();
    setDraggingIndex(index);
  }, [pipelineActive]);

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
    if (!imgRef.current || !effectiveSnapshot.naturalDims || corners.length === 0) return null;
    const rect = imgRef.current.getBoundingClientRect();
    const sx = rect.width / effectiveSnapshot.naturalDims.w;
    const sy = rect.height / effectiveSnapshot.naturalDims.h;
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
            fill="color-mix(in srgb, var(--color-accent) 15%, transparent)"
            stroke="var(--color-accent)"
            strokeWidth="2"
            strokeDasharray={scaled.length < 4 ? '6 3' : undefined}
          />
        )}
        {scaled.map(([x, y], i) => {
          const label = sorted ? CORNER_LABELS[i] : String(i + 1);
          return (
            <g
              key={`corner-${label}`}
              style={{ pointerEvents: 'all', cursor: draggingIndex === i ? 'grabbing' : 'grab' }}
              onMouseDown={(e) => handleDotMouseDown(e, i)}
            >
              <circle cx={x} cy={y} r={DOT_HIT_RADIUS} fill="transparent" />
              <circle cx={x} cy={y} r={DOT_RADIUS} fill="var(--color-accent)" stroke="var(--color-accent-fg)" strokeWidth="2" />
              <text
                x={x}
                y={Number(y) + 1}
                textAnchor="middle"
                dominantBaseline="central"
                fill="var(--color-accent-fg)"
                fontSize="10"
                fontWeight="bold"
              >
                {label}
              </text>
            </g>
          );
        })}
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
              onLoad={(e) => {
                const img = e.currentTarget;
                if (img.naturalWidth > 0 && img.naturalHeight > 0) {
                  const dims = { w: img.naturalWidth, h: img.naturalHeight };
                  setSnapshot((prev) => ({ ...prev, naturalDims: dims }));
                  onSnapshotDimsChange?.(dims);
                } else {
                  // Browser fired onLoad but couldn't read dims — treat as error so
                  // the loading spinner clears (naturalDims stays null, error clears it).
                  setSnapshot((prev) => ({ ...prev, error: 'Snapshot loaded with unknown dimensions' }));
                }
              }}
              onError={() => {
                setSnapshot((prev) => ({ ...prev, error: 'Failed to load snapshot' }));
              }}
              draggable={false}
            />
            {/* eslint-disable-next-line react-hooks/refs -- overlay reads
                imgRef.current to size the SVG after first paint; safe per
                React docs since the ref is set by the time renderOverlay
                runs on subsequent renders */}
            {renderOverlay()}
            {snapshotSourceId && pipelineActive && (
              <button
                onClick={retakeSnapshot}
                aria-label="Retake snapshot"
                className="absolute top-2 right-2 z-20 p-1.5 bg-surface-overlay hover:bg-surface-overlay rounded text-fg-inverse transition focus-visible:ring-2 focus-visible:ring-focus-ring"
                title="Retake snapshot"
              >
                <ArrowPathIcon className={ICON_SIZE.SM} />
              </button>
            )}
          </div>
        )}
        {!loading && !snapshotUrl && !effectiveSnapshot.error && (
          <div className="flex items-center justify-center h-64">
            <div className="text-fg-subtle">No snapshot available</div>
          </div>
        )}
        {effectiveSnapshot.error && !loading && (
          <div className="flex items-center justify-center h-64">
            <div className="text-danger">{effectiveSnapshot.error}</div>
          </div>
        )}
      </div>

      {corners.length > 0 && (
        <div className="flex flex-wrap gap-3 mb-4 text-sm font-mono text-fg-muted">
          {corners.map(([x, y], i) => {
            const label = sorted ? CORNER_LABELS[i] : String(i + 1);
            return (
              <span key={`coord-${label}`} className="px-2 py-1 bg-surface-muted rounded">
                {label}({x},{y})
              </span>
            );
          })}
        </div>
      )}
    </>
  );
}
