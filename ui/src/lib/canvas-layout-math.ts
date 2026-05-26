import type { CanvasDims, LayoutSlot } from './composer-types';

export type HandlePos =
  | 'nw'
  | 'n'
  | 'ne'
  | 'e'
  | 'se'
  | 's'
  | 'sw'
  | 'w'
  | 'move';

export const ALIGN_THRESHOLD = 6;

export const CORNER_HANDLES: ReadonlySet<HandlePos> = new Set(['nw', 'ne', 'sw', 'se']);

export function lockCornerAspect(
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

export function applyMinSize(
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

function boundsForMove(
  x: number, y: number, w: number, h: number, canvas: CanvasDims,
): { x: number; y: number; w: number; h: number } {
  if (x < 0) x = 0;
  if (y < 0) y = 0;
  if (x + w > canvas.w) x = canvas.w - w;
  if (y + h > canvas.h) y = canvas.h - h;
  return { x, y, w, h };
}

function boundsForResize(
  x: number, y: number, w: number, h: number, canvas: CanvasDims,
): { x: number; y: number; w: number; h: number } {
  if (x < 0) { w += x; x = 0; }
  if (y < 0) { h += y; y = 0; }
  if (x + w > canvas.w) w = canvas.w - x;
  if (y + h > canvas.h) h = canvas.h - y;
  return { x, y, w, h };
}

export function applyCanvasBounds(
  x: number, y: number, w: number, h: number,
  canvas: CanvasDims, handle: HandlePos,
): { x: number; y: number; w: number; h: number } {
  return handle === 'move'
    ? boundsForMove(x, y, w, h, canvas)
    : boundsForResize(x, y, w, h, canvas);
}

export function applyHandleDelta(
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
  return { ...start, ...roundForHandle(x, y, w, h, handle) };
}

function roundForHandle(
  x: number, y: number, w: number, h: number, handle: HandlePos,
): { x: number; y: number; w: number; h: number } {
  if (handle === 'move') return { x: Math.round(x), y: Math.round(y), w: Math.round(w), h: Math.round(h) };
  const fixedRight = handle === 'w' || handle === 'nw' || handle === 'sw';
  const fixedBottom = handle === 'n' || handle === 'nw' || handle === 'ne';
  const fixedLeft = handle === 'e' || handle === 'ne' || handle === 'se';
  const fixedTop = handle === 's' || handle === 'sw' || handle === 'se';
  let rx: number, rw: number, ry: number, rh: number;
  if (fixedRight) { const right = Math.round(x + w); rx = Math.round(x); rw = right - rx; }
  else if (fixedLeft) { rx = Math.round(x); rw = Math.round(x + w) - rx; }
  else { rx = Math.round(x); rw = Math.round(w); }
  if (fixedBottom) { const bottom = Math.round(y + h); ry = Math.round(y); rh = bottom - ry; }
  else if (fixedTop) { ry = Math.round(y); rh = Math.round(y + h) - ry; }
  else { ry = Math.round(y); rh = Math.round(h); }
  return { x: rx, y: ry, w: rw, h: rh };
}

export function snap(value: number, grid: number): number {
  return Math.round(value / grid) * grid;
}

export function snapSlot(slot: LayoutSlot, grid: number, handle: HandlePos = 'move'): LayoutSlot {
  if (handle === 'move') {
    return { ...slot, x: snap(slot.x, grid), y: snap(slot.y, grid), w: snap(slot.w, grid), h: snap(slot.h, grid) };
  }
  const anchorsRight = handle === 'w' || handle === 'nw' || handle === 'sw';
  const anchorsBottom = handle === 'n' || handle === 'nw' || handle === 'ne';
  const anchorsLeft = handle === 'e' || handle === 'ne' || handle === 'se';
  const anchorsTop = handle === 's' || handle === 'sw' || handle === 'se';
  let { x, y, w, h } = slot;
  if (anchorsRight) { const right = x + w; x = snap(x, grid); w = right - x; }
  else if (anchorsLeft) { w = snap(x + w, grid) - x; }
  else { x = snap(x, grid); w = snap(w, grid); }
  if (anchorsBottom) { const bottom = y + h; y = snap(y, grid); h = bottom - y; }
  else if (anchorsTop) { h = snap(y + h, grid) - y; }
  else { y = snap(y, grid); h = snap(h, grid); }
  return { ...slot, x, y, w, h };
}

export function clampToCanvas(slot: LayoutSlot, canvas: CanvasDims, handle: HandlePos): LayoutSlot {
  return handle === 'move'
    ? clampMove(slot, canvas)
    : clampResize(slot, canvas);
}

export function clampMove(slot: LayoutSlot, canvas: CanvasDims): LayoutSlot {
  let { x, y } = slot;
  const { w, h } = slot;
  if (x < 0) x = 0;
  if (y < 0) y = 0;
  if (x + w > canvas.w) x = canvas.w - w;
  if (y + h > canvas.h) y = canvas.h - h;
  return { ...slot, x, y };
}

export function clampResize(slot: LayoutSlot, canvas: CanvasDims): LayoutSlot {
  let { x, y, w, h } = slot;
  if (x < 0) { w += x; x = 0; }
  if (y < 0) { h += y; y = 0; }
  if (x + w > canvas.w) w = canvas.w - x;
  if (y + h > canvas.h) h = canvas.h - y;
  return { ...slot, x, y, w, h };
}

export interface AlignmentGuide {
  axis: 'x' | 'y';
  value: number;
}

export function buildAlignmentCandidates(
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

export function applyAlignment(
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
