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

export function findAlignmentSnap(
  pos: number,
  size: number,
  candidates: number[],
): { snappedPos: number; guidePos: number } | null {
  const ALIGN_THRESHOLD = 6;
  let best: { snappedPos: number; guidePos: number; dist: number } | null =
    null;
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
