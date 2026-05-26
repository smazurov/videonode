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

const CORNER_HANDLES: ReadonlySet<HandlePos> = new Set(['nw', 'ne', 'sw', 'se']);

// Snap a corner drag back onto the start aspect ratio. Pick the
// dominant axis (the one the user has moved further as a fraction of
// the slot's original size) and derive the other dimension; then
// re-anchor x/y so the opposite corner stays fixed.
function lockCornerAspect(
  start: LayoutSlot,
  handle: HandlePos,
  dx: number,
  dy: number,
  freeW: number,
  freeH: number,
): { x: number; y: number; w: number; h: number } {
  const { x: sx, y: sy, w: sw, h: sh } = start;
  const aspect = sw / Math.max(1, sh);
  const fracX = Math.abs(dx) / Math.max(1, sw);
  const fracY = Math.abs(dy) / Math.max(1, sh);
  const w = fracX >= fracY ? freeW : freeH * aspect;
  const h = fracX >= fracY ? freeW / aspect : freeH;
  switch (handle) {
    case 'nw':
      return { x: sx + sw - w, y: sy + sh - h, w, h };
    case 'ne':
      return { x: sx, y: sy + sh - h, w, h };
    case 'sw':
      return { x: sx + sw - w, y: sy, w, h };
    case 'se':
      return { x: sx, y: sy, w, h };
    default:
      return { x: sx, y: sy, w, h };
  }
}

function applyMinSize(
  x: number, y: number, w: number, h: number,
  start: LayoutSlot, handle: HandlePos, aspectLock: boolean,
): { x: number; y: number; w: number; h: number } {
  if (w >= 16 && h >= 16) return { x, y, w, h };
  const isCornerLock = aspectLock && CORNER_HANDLES.has(handle);
  if (!isCornerLock) return { x, y, w: Math.max(16, w), h: Math.max(16, h) };
  const aspect = start.w / Math.max(1, start.h);
  if (w < 16) { w = 16; h = w / aspect; }
  if (h < 16) { h = 16; w = h * aspect; }
  const anchorsLeft = handle === 'nw' || handle === 'sw';
  const anchorsTop = handle === 'nw' || handle === 'ne';
  return {
    x: anchorsLeft ? start.x + start.w - w : x,
    y: anchorsTop ? start.y + start.h - h : y,
    w, h,
  };
}

function applyCanvasBounds(
  x: number, y: number, w: number, h: number,
  canvas: CanvasDims, handle: HandlePos,
): { x: number; y: number; w: number; h: number } {
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
  return { x, y, w, h };
}

function applyHandleDelta(
  start: LayoutSlot,
  handle: HandlePos,
  dx: number,
  dy: number,
  canvas: CanvasDims,
  aspectLock: boolean,
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
  if (aspectLock && CORNER_HANDLES.has(handle)) {
    ({ x, y, w, h } = lockCornerAspect(start, handle, dx, dy, w, h));
  }
  ({ x, y, w, h } = applyMinSize(x, y, w, h, start, handle, aspectLock));
  ({ x, y, w, h } = applyCanvasBounds(x, y, w, h, canvas, handle));
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

function clampToCanvas(slot: LayoutSlot, canvas: CanvasDims, handle: HandlePos): LayoutSlot {
  return handle === 'move'
    ? clampMove(slot, canvas)
    : clampResize(slot, canvas);
}

function clampMove(slot: LayoutSlot, canvas: CanvasDims): LayoutSlot {
  let { x, y } = slot;
  const { w, h } = slot;
  if (x < 0) x = 0;
  if (y < 0) y = 0;
  if (x + w > canvas.w) x = canvas.w - w;
  if (y + h > canvas.h) y = canvas.h - h;
  return { ...slot, x, y };
}

function clampResize(slot: LayoutSlot, canvas: CanvasDims): LayoutSlot {
  let { x, y, w, h } = slot;
  if (x < 0) { w += x; x = 0; }
  if (y < 0) { h += y; y = 0; }
  if (x + w > canvas.w) w = canvas.w - x;
  if (y + h > canvas.h) h = canvas.h - y;
  return { ...slot, x, y, w, h };
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

  const borderPx = 4; // border-2 = 2px each side
  const contentW = Math.max(1, displayW - borderPx);
  const aspect = canvas.w / Math.max(1, canvas.h);
  const contentH = contentW / aspect;
  const scale = contentW / Math.max(1, canvas.w);

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
      // Aspect-ratio lock is the default on corner drags. Holding Ctrl
      // (or ⌘ on macOS) bypasses the lock and lets the user free-resize.
      // When locked, snap-to-grid and alignment guides are suppressed
      // because both can break the ratio.
      const aspectLock = CORNER_HANDLES.has(drag.handle) && !(e.ctrlKey || e.metaKey);
      let next = applyHandleDelta(drag.startSlot, drag.handle, dx, dy, canvas, aspectLock);
      if (!aspectLock && snapToGrid && gridSize > 0) next = clampToCanvas(snapSlot(next, gridSize), canvas, drag.handle);
      if (aspectLock) {
        setGuides([]);
        updateSlot(drag.slotInput, () => next);
        return;
      }
      const others = layoutRef.current.filter((s) => s.input !== drag.slotInput);
      const candidates = buildAlignmentCandidates(canvas, next, others);
      const aligned = applyAlignment(next, candidates);
      setGuides(aligned.guides);
      updateSlot(drag.slotInput, () => clampToCanvas(aligned.slot, canvas, drag.handle));
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
      updateSlot(selectedInput, (s) => {
        let nx = s.x + dx;
        let ny = s.y + dy;
        if (snapToGrid && gridSize > 0) {
          nx = snap(nx, gridSize);
          ny = snap(ny, gridSize);
        }
        return {
          ...s,
          x: Math.max(0, Math.min(canvas.w - s.w, nx)),
          y: Math.max(0, Math.min(canvas.h - s.h, ny)),
        };
      });
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [canvas.h, canvas.w, gridSize, selectedInput, snapToGrid, updateSlot]);

  // Slot rectangles in screen-px for handle overlays.
  const handleOffset = borderPx / 2;
  const screenRects = useMemo(
    () =>
      layout.map((slot) => ({
        slot,
        left: slot.x * scale + handleOffset,
        top: slot.y * scale + handleOffset,
        width: slot.w * scale,
        height: slot.h * scale,
      })),
    [layout, scale, handleOffset],
  );

  return (
    <div ref={containerRef} className="select-none min-w-0 overflow-hidden">
      {showRulers && contentW > 1 && (
        <div className="flex items-start" style={{ paddingLeft: 20 + handleOffset }}>
          <CanvasRuler size={canvas.w} displaySize={contentW} orientation="horizontal" />
        </div>
      )}
      <div className="flex items-start">
        {showRulers && contentH > 1 && (
          <CanvasRuler size={canvas.h} displaySize={contentH} orientation="vertical" />
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
            hideCaption
          />
          {/* Handle overlay: one absolute box per slot in screen-px. */}
          {displayW > 0 && <div className="absolute inset-0">
            {screenRects.map(({ slot, left, top, width, height }) => {
              const isSelected = selectedInput === slot.input;
              return (
                <div
                  key={slot.input}
                  className={`absolute ${isSelected ? 'ring-2 ring-inset ring-accent' : ''}`}
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
          </div>}
          {/* Alignment guides — drawn over the canvas in screen-px. */}
          {guides.length > 0 && contentW > 1 && (
            <svg
              className="absolute pointer-events-none"
              style={{ left: handleOffset, top: handleOffset }}
              width={contentW}
              height={contentH}
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
                      y2={contentH}
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
                    x2={contentW}
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
