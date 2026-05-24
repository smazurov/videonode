import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from 'react';

import { CanvasPreview } from '../CanvasPreview';
import { CanvasRuler } from './CanvasRuler';
import { LayoutSlotHandle, type HandlePos } from './LayoutSlotHandle';
import type {
  CanvasDims,
  ComposerInput,
  LayoutSlot,
} from '../../lib/composer-types';

interface CanvasEditorProps {
  canvas: CanvasDims;
  inputs: readonly ComposerInput[];
  layout: readonly LayoutSlot[];
  selectedInput: string | null;
  onSelect: (input: string | null) => void;
  /**
   * Called on every interaction step (drag move, key nudge). Implementations
   * are expected to debounce the network save themselves (see composer-layout
   * route) — this hook fires aggressively so the UI stays live.
   */
  onLayoutChange: (next: LayoutSlot[]) => void;
  gridSize?: number;
  snapToGrid?: boolean;
  showRulers?: boolean;
  saving?: boolean;
}

interface DragState {
  pointerId: number;
  origin: { x: number; y: number };
  startSlot: LayoutSlot;
  handle: HandlePos;
  slotInput: string;
}

const ALIGN_THRESHOLD = 6; // canvas-px snap distance for alignment guides.
const NUDGE_STEP = 1;
const NUDGE_STEP_LARGE = 10;

function applyHandleDelta(
  start: LayoutSlot,
  handle: HandlePos,
  dx: number,
  dy: number,
  canvas: CanvasDims,
): LayoutSlot {
  const { x: sx, y: sy, w: sw, h: sh } = start;
  let x = sx;
  let y = sy;
  let w = sw;
  let h = sh;
  switch (handle) {
    case 'move':
      x = sx + dx;
      y = sy + dy;
      break;
    case 'n':
      y = sy + dy;
      h = sh - dy;
      break;
    case 's':
      h = sh + dy;
      break;
    case 'e':
      w = sw + dx;
      break;
    case 'w':
      x = sx + dx;
      w = sw - dx;
      break;
    case 'nw':
      x = sx + dx;
      y = sy + dy;
      w = sw - dx;
      h = sh - dy;
      break;
    case 'ne':
      y = sy + dy;
      w = sw + dx;
      h = sh - dy;
      break;
    case 'sw':
      x = sx + dx;
      w = sw - dx;
      h = sh + dy;
      break;
    case 'se':
      w = sw + dx;
      h = sh + dy;
      break;
  }
  if (w < 16) w = 16;
  if (h < 16) h = 16;
  if (x < 0) x = 0;
  if (y < 0) y = 0;
  const { w: cw, h: ch } = canvas;
  if (x + w > cw) {
    if (handle === 'move') x = cw - w;
    else w = cw - x;
  }
  if (y + h > ch) {
    if (handle === 'move') y = ch - h;
    else h = ch - y;
  }
  return { ...start, x: Math.round(x), y: Math.round(y), w: Math.round(w), h: Math.round(h) };
}

function snap(value: number, grid: number): number {
  return Math.round(value / grid) * grid;
}

function snapSlot(slot: LayoutSlot, grid: number): LayoutSlot {
  const { x, y, w, h } = slot;
  return {
    ...slot,
    x: snap(x, grid),
    y: snap(y, grid),
    w: snap(w, grid),
    h: snap(h, grid),
  };
}

interface AlignmentGuide {
  axis: 'x' | 'y';
  value: number;
}

// Build a list of alignment candidates from canvas edges + other-slot edges.
function buildAlignmentCandidates(
  canvas: CanvasDims,
  slot: LayoutSlot,
  others: readonly LayoutSlot[],
): { xs: number[]; ys: number[] } {
  const { w: cw, h: ch } = canvas;
  const { input: slotInput } = slot;
  const xs: number[] = [0, cw / 2, cw];
  const ys: number[] = [0, ch / 2, ch];
  for (const other of others) {
    const { input, x, y, w, h } = other;
    if (input === slotInput) continue;
    xs.push(x, x + w, x + w / 2);
    ys.push(y, y + h, y + h / 2);
  }
  return { xs, ys };
}

function applyAlignment(
  slot: LayoutSlot,
  candidates: { xs: number[]; ys: number[] },
): { slot: LayoutSlot; guides: AlignmentGuide[] } {
  const { x: sx, y: sy, w: sw, h: sh } = slot;
  const guides: AlignmentGuide[] = [];
  let x = sx;
  let y = sy;
  const left = sx;
  const right = sx + sw;
  const centerX = sx + sw / 2;
  const top = sy;
  const bottom = sy + sh;
  const centerY = sy + sh / 2;
  for (const cx of candidates.xs) {
    if (Math.abs(left - cx) <= ALIGN_THRESHOLD) {
      x = cx;
      guides.push({ axis: 'x', value: cx });
      break;
    }
    if (Math.abs(right - cx) <= ALIGN_THRESHOLD) {
      x = cx - sw;
      guides.push({ axis: 'x', value: cx });
      break;
    }
    if (Math.abs(centerX - cx) <= ALIGN_THRESHOLD) {
      x = cx - sw / 2;
      guides.push({ axis: 'x', value: cx });
      break;
    }
  }
  for (const cy of candidates.ys) {
    if (Math.abs(top - cy) <= ALIGN_THRESHOLD) {
      y = cy;
      guides.push({ axis: 'y', value: cy });
      break;
    }
    if (Math.abs(bottom - cy) <= ALIGN_THRESHOLD) {
      y = cy - sh;
      guides.push({ axis: 'y', value: cy });
      break;
    }
    if (Math.abs(centerY - cy) <= ALIGN_THRESHOLD) {
      y = cy - sh / 2;
      guides.push({ axis: 'y', value: cy });
      break;
    }
  }
  return { slot: { ...slot, x: Math.round(x), y: Math.round(y) }, guides };
}

export function CanvasEditor({
  canvas,
  inputs,
  layout,
  selectedInput,
  onSelect,
  onLayoutChange,
  gridSize = 10,
  snapToGrid = true,
  showRulers = true,
  saving = false,
}: Readonly<CanvasEditorProps>) {
  const containerRef = useRef<HTMLDivElement>(null);
  const surfaceRef = useRef<HTMLDivElement>(null);
  const [displayW, setDisplayW] = useState(0);

  const dragRef = useRef<DragState | null>(null);
  const [guides, setGuides] = useState<AlignmentGuide[]>([]);

  // Measure the display width of the editor surface so handles can be placed
  // in screen-px and we can derive an accurate canvas-px scale.
  useLayoutEffect(() => {
    if (!surfaceRef.current) return;
    const el = surfaceRef.current;
    const ro = new ResizeObserver(() => {
      setDisplayW(el.clientWidth);
    });
    ro.observe(el);
    setDisplayW(el.clientWidth);
    return () => ro.disconnect();
  }, []);

  const aspect = canvas.w / Math.max(1, canvas.h);
  const displayH = displayW / aspect;
  const scale = displayW / Math.max(1, canvas.w);

  // Stable accessor — handlers want the current layout without retriggering.
  const layoutRef = useRef(layout);
  useEffect(() => {
    layoutRef.current = layout;
  }, [layout]);

  const commitLayout = useCallback(
    (next: LayoutSlot[]) => {
      onLayoutChange(next);
    },
    [onLayoutChange],
  );

  const updateSlot = useCallback(
    (input: string, mutator: (s: LayoutSlot) => LayoutSlot) => {
      const next = layoutRef.current.map((s) => (s.input === input ? mutator(s) : s));
      commitLayout(next);
    },
    [commitLayout],
  );

  const handlePointerDown = useCallback(
    (e: React.PointerEvent<HTMLDivElement>, position: HandlePos, slot: LayoutSlot) => {
      if (e.button !== 0) return;
      e.preventDefault();
      (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
      dragRef.current = {
        pointerId: e.pointerId,
        origin: { x: e.clientX, y: e.clientY },
        startSlot: slot,
        handle: position,
        slotInput: slot.input,
      };
      onSelect(slot.input);
    },
    [onSelect],
  );

  const handlePointerMove = useCallback(
    (e: React.PointerEvent<HTMLDivElement>) => {
      const drag = dragRef.current;
      if (!drag || drag.pointerId !== e.pointerId) return;
      const dxScreen = e.clientX - drag.origin.x;
      const dyScreen = e.clientY - drag.origin.y;
      const dx = dxScreen / Math.max(scale, 0.0001);
      const dy = dyScreen / Math.max(scale, 0.0001);
      let next = applyHandleDelta(drag.startSlot, drag.handle, dx, dy, canvas);
      if (snapToGrid && gridSize > 0) next = snapSlot(next, gridSize);
      const others = layoutRef.current.filter((s) => s.input !== drag.slotInput);
      const candidates = buildAlignmentCandidates(canvas, next, others);
      const aligned = applyAlignment(next, candidates);
      setGuides(aligned.guides);
      updateSlot(drag.slotInput, () => aligned.slot);
    },
    [canvas, gridSize, scale, snapToGrid, updateSlot],
  );

  const handlePointerUp = useCallback(
    (e: React.PointerEvent<HTMLDivElement>) => {
      const drag = dragRef.current;
      if (!drag || drag.pointerId !== e.pointerId) return;
      try {
        (e.currentTarget as HTMLElement).releasePointerCapture(e.pointerId);
      } catch {
        // releasePointerCapture can throw if pointer already released; safe to ignore.
      }
      dragRef.current = null;
      setGuides([]);
    },
    [],
  );

  // Keyboard nudge: arrow keys move the selected slot. Shift = ×10.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!selectedInput) return;
      const tag = (e.target as HTMLElement | null)?.tagName?.toLowerCase();
      if (tag === 'input' || tag === 'textarea' || tag === 'select') return;
      const step = e.shiftKey ? NUDGE_STEP_LARGE : NUDGE_STEP;
      let dx = 0;
      let dy = 0;
      switch (e.key) {
        case 'ArrowLeft':
          dx = -step;
          break;
        case 'ArrowRight':
          dx = step;
          break;
        case 'ArrowUp':
          dy = -step;
          break;
        case 'ArrowDown':
          dy = step;
          break;
        default:
          return;
      }
      e.preventDefault();
      updateSlot(selectedInput, (s) => ({
        ...s,
        x: Math.max(0, Math.min(canvas.w - s.w, s.x + dx)),
        y: Math.max(0, Math.min(canvas.h - s.h, s.y + dy)),
      }));
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [canvas.h, canvas.w, selectedInput, updateSlot]);

  // Slot rectangles in screen-px for handle overlays.
  const screenRects = useMemo(
    () =>
      layout.map((slot) => ({
        slot,
        left: slot.x * scale,
        top: slot.y * scale,
        width: slot.w * scale,
        height: slot.h * scale,
      })),
    [layout, scale],
  );

  return (
    <div ref={containerRef} className="select-none">
      {showRulers && displayW > 0 && (
        <div className="flex items-start pl-5">
          <CanvasRuler size={canvas.w} displaySize={displayW} orientation="horizontal" />
        </div>
      )}
      <div className="flex items-start">
        {showRulers && displayH > 0 && (
          <CanvasRuler size={canvas.h} displaySize={displayH} orientation="vertical" />
        )}
        <div
          ref={surfaceRef}
          className="relative flex-1 min-w-0"
          onPointerMove={handlePointerMove}
          onPointerUp={handlePointerUp}
          onPointerCancel={handlePointerUp}
          onClick={(e) => {
            if (e.target === e.currentTarget) onSelect(null);
          }}
        >
          <CanvasPreview
            canvas={canvas}
            inputs={inputs}
            layout={layout}
            selectedInput={selectedInput}
            loading={saving}
            hideCaption
          />
          {/* Handle overlay: one absolute box per slot in screen-px. */}
          <div className="absolute inset-0">
            {screenRects.map(({ slot, left, top, width, height }) => {
              const isSelected = selectedInput === slot.input;
              return (
                <div
                  key={slot.input}
                  className={`absolute ${isSelected ? 'ring-2 ring-accent' : ''}`}
                  style={{ left, top, width, height }}
                  onPointerDown={(e) => handlePointerDown(e, 'move', slot)}
                >
                  <LayoutSlotHandle
                    position="move"
                    onPointerDown={(e) => handlePointerDown(e, 'move', slot)}
                  />
                  {isSelected &&
                    (['nw', 'n', 'ne', 'e', 'se', 's', 'sw', 'w'] as const).map((pos) => (
                      <LayoutSlotHandle
                        key={pos}
                        position={pos}
                        onPointerDown={(e) => handlePointerDown(e, pos, slot)}
                      />
                    ))}
                </div>
              );
            })}
          </div>
          {/* Alignment guides — drawn over the canvas in screen-px. */}
          {guides.length > 0 && displayW > 0 && (
            <svg
              className="absolute inset-0 pointer-events-none"
              width={displayW}
              height={displayH}
            >
              {guides.map((g, i) => {
                if (g.axis === 'x') {
                  const x = g.value * scale;
                  return (
                    <line
                      key={`g-${i}`}
                      x1={x}
                      y1={0}
                      x2={x}
                      y2={displayH}
                      stroke="#22c55e"
                      strokeWidth={1}
                      strokeDasharray="4 3"
                    />
                  );
                }
                const y = g.value * scale;
                return (
                  <line
                    key={`g-${i}`}
                    x1={0}
                    y1={y}
                    x2={displayW}
                    y2={y}
                    stroke="#22c55e"
                    strokeWidth={1}
                    strokeDasharray="4 3"
                  />
                );
              })}
            </svg>
          )}
        </div>
      </div>
    </div>
  );
}
