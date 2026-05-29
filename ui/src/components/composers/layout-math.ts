export interface ArPreview {
  x: number;
  y: number;
  w: number;
  h: number;
}

export function computeArPreview(
  slotW: number, slotH: number,
  srcW: number, srcH: number,
  mode: string | undefined,
  cropX = 0.5, cropY = 0.5,
  cropScale = 1.0,
): ArPreview | null {
  if (!mode || mode === 'stretch') return null;
  if (srcW <= 0 || srcH <= 0 || slotW <= 0 || slotH <= 0) return null;

  const srcAr = srcW / srcH;
  const slotAr = slotW / slotH;

  if (mode === 'fit') {
    if (srcAr > slotAr) {
      const h = slotW / srcAr;
      return { x: 0, y: (slotH - h) / 2, w: slotW, h };
    }
    const w = slotH * srcAr;
    return { x: (slotW - w) / 2, y: 0, w, h: slotH };
  }

  if (mode === 'crop') {
    const fillScale = Math.max(slotW / srcW, slotH / srcH);
    const s = Math.max(1, cropScale);
    const scaledW = srcW * fillScale * s;
    const scaledH = srcH * fillScale * s;
    const excessX = scaledW - slotW;
    const excessY = scaledH - slotH;
    const ox = clamp(cropX, 0, 1);
    const oy = clamp(cropY, 0, 1);
    return { x: -excessX * ox, y: -excessY * oy, w: scaledW, h: scaledH };
  }

  return null;
}

export function computeCropPercent(
  slotW: number, slotH: number,
  srcW: number, srcH: number,
): number {
  if (srcW <= 0 || srcH <= 0 || slotW <= 0 || slotH <= 0) return 100;
  const srcAr = srcW / srcH;
  const slotAr = slotW / slotH;
  if (srcAr > slotAr) return Math.max(1, Math.round((slotAr / srcAr) * 100));
  return Math.max(1, Math.round((srcAr / slotAr) * 100));
}

export function snapVal(v: number, grid: number): number {
  return Math.round(v / grid) * grid;
}

export function clamp(v: number, lo: number, hi: number): number {
  return Math.max(lo, Math.min(hi, v));
}

export interface DefaultSlotDims {
  input: string;
  x: number;
  y: number;
  w: number;
  h: number;
}

export function makeDefaultLayoutSlot(
  ref: string,
  canvas: { w: number; h: number },
  srcDims?: { w: number; h: number },
): DefaultSlotDims {
  const halfW = Math.round(canvas.w / 2);
  const halfH = Math.round(canvas.h / 2);
  let w = halfW;
  let h = halfH;
  if (srcDims && srcDims.w > 0 && srcDims.h > 0) {
    const ar = srcDims.w / srcDims.h;
    h = Math.round(w / ar);
    if (h > halfH) {
      h = halfH;
      w = Math.round(h * ar);
    }
  }
  return {
    input: ref,
    x: Math.max(0, Math.round((canvas.w - w) / 2)),
    y: Math.max(0, Math.round((canvas.h - h) / 2)),
    w,
    h,
  };
}

export function visualToKonva(
  vx: number, vy: number, w: number, h: number, rotation: number,
): { x: number; y: number } {
  switch (((rotation % 360) + 360) % 360) {
    case 90:  return { x: vx + h, y: vy };
    case 180: return { x: vx + w, y: vy + h };
    case 270: return { x: vx,     y: vy + w };
    default:  return { x: vx,     y: vy };
  }
}

export function konvaToVisual(
  kx: number, ky: number, w: number, h: number, rotation: number,
): { x: number; y: number } {
  switch (((rotation % 360) + 360) % 360) {
    case 90:  return { x: kx - h, y: ky };
    case 180: return { x: kx - w, y: ky - h };
    case 270: return { x: kx,     y: ky - w };
    default:  return { x: kx,     y: ky };
  }
}

export function visualDims(w: number, h: number, rotation: number): { vw: number; vh: number } {
  return (rotation % 180 !== 0) ? { vw: h, vh: w } : { vw: w, vh: h };
}

/**
 * Rotate a child offset from a rotated group's local frame into the parent's
 * world frame. Konva rotates clockwise about the node origin in a y-down space,
 * so for a local offset (ox, oy) the world delta is (ox·cosθ − oy·sinθ,
 * ox·sinθ + oy·cosθ). Needed when a resize leaves a rect offset inside a
 * rotated group: the offset must be world-rotated before adding to group.x/y.
 */
export function rotateOffset(
  ox: number, oy: number, rotation: number,
): { dx: number; dy: number } {
  switch (((rotation % 360) + 360) % 360) {
    case 90:  return { dx: -oy, dy: ox };
    case 180: return { dx: -ox, dy: -oy };
    case 270: return { dx: oy,  dy: -ox };
    default:  return { dx: ox,  dy: oy };
  }
}

export function findAlignmentSnap(
  pos: number, size: number, candidates: number[],
): { snappedPos: number; guidePos: number } | null {
  const ALIGN_THRESHOLD = 6;
  let best: { snappedPos: number; guidePos: number; dist: number } | null = null;
  for (const c of candidates) {
    const edges = [
      { snappedPos: c, dist: Math.abs(pos - c) },
      { snappedPos: c - size, dist: Math.abs(pos + size - c) },
      { snappedPos: c - size / 2, dist: Math.abs(pos + size / 2 - c) },
    ];
    for (const e of edges) {
      if (e.dist <= ALIGN_THRESHOLD && (!best || e.dist < best.dist)) {
        best = { snappedPos: e.snappedPos, guidePos: c, dist: e.dist };
      }
    }
  }
  return best ? { snappedPos: best.snappedPos, guidePos: best.guidePos } : null;
}
