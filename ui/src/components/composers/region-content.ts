// Region/Content geometry — the editor's domain model.
//
// Region: the layout output rectangle on the canvas (x,y,w,h). Axis-aligned,
//   never rotates, clamped to the canvas. Clips its content.
// Content: the source overlay inside a region. Carries the source AR, ROTATES
//   (0/90/180/270), fills the region via cover/fit/stretch, is clipped to the
//   region (not the canvas), and is panned/zoomed in cover mode.
//
// This module is framework-free: no React, no Konva at runtime. Konva is used
// only as a test oracle (its getClientRect/Transform cross-checks the math).

import type {
  AspectRatioMode,
  CropConfig,
  LayoutSlot,
} from "../../lib/composer-types";
import { clamp, snapVal } from "./layout-math";

// Re-export the geometry helpers shared with the editor so callers import from one module.
export { clamp, snapVal, findAlignmentSnap } from "./layout-math";

export interface Rect {
  x: number;
  y: number;
  w: number;
  h: number;
}
export interface Size {
  w: number;
  h: number;
}
export type Rotation = 0 | 90 | 180 | 270;
export type FillMode = "stretch" | "fit" | "cover";

export interface ContentTransform {
  rotation: Rotation;
  fill: FillMode;
  panX: number; // normalized 0..1 (== backend crop_x); 0.5 = centered
  panY: number; // normalized 0..1 (== backend crop_y)
  zoom: number; // >= 1 (== backend crop_scale)
}

export type ResizeHandle =
  | "top-left"
  | "top-center"
  | "top-right"
  | "middle-left"
  | "middle-right"
  | "bottom-left"
  | "bottom-center"
  | "bottom-right";

export const MIN_REGION_SIZE = 16;

const movesLeft = (h: ResizeHandle) =>
  h === "top-left" || h === "middle-left" || h === "bottom-left";
const movesRight = (h: ResizeHandle) =>
  h === "top-right" || h === "middle-right" || h === "bottom-right";
const movesTop = (h: ResizeHandle) =>
  h === "top-left" || h === "top-center" || h === "top-right";
const movesBottom = (h: ResizeHandle) =>
  h === "bottom-left" || h === "bottom-center" || h === "bottom-right";

/** Rotation-adjusted source aspect ratio. 90/270 invert w:h. */
export function effectiveAr(
  srcW: number,
  srcH: number,
  rotation: Rotation,
): number {
  const ar = srcW / srcH;
  return rotation % 180 === 0 ? ar : 1 / ar;
}

/**
 * Clamp a region's position so the whole box stays inside the canvas. Move
 * only — never changes size. Optional grid snap is applied before clamping.
 * If the region is larger than the canvas on an axis, it pins to 0 on that axis.
 */
export function clampRegionMove(
  region: Rect,
  canvas: Size,
  doSnap: boolean,
  grid: number,
): Rect {
  let x = region.x;
  let y = region.y;
  if (doSnap && grid > 0) {
    x = snapVal(x, grid);
    y = snapVal(y, grid);
  }
  return {
    x: clamp(x, 0, canvas.w - region.w),
    y: clamp(y, 0, canvas.h - region.h),
    w: region.w,
    h: region.h,
  };
}

/**
 * Resize a region from a transform gesture. The anchored corner/edge (opposite
 * the dragged handle) stays FIXED; the box only grows or shrinks toward the
 * handle and is CAPPED at the canvas edge — it is never relocated. Corner
 * handles preserve aspect ratio; edge handles resize a single axis freely.
 */
export function resizeRegion(args: {
  region: Rect;
  handle: ResizeHandle;
  canvas: Size;
  proposed: Rect;
  lockAspect: boolean;
  minSize?: number;
  doSnap?: boolean;
  grid?: number;
}): Rect {
  const {
    region,
    handle,
    canvas,
    proposed,
    lockAspect,
    minSize = MIN_REGION_SIZE,
    doSnap = false,
    grid: gridArg = 0,
  } = args;
  const grid = doSnap ? gridArg : 0;
  const mL = movesLeft(handle);
  const mR = movesRight(handle);
  const mT = movesTop(handle);
  const mB = movesBottom(handle);

  if (lockAspect && (mL || mR) && (mT || mB)) {
    return resizeCorner(region, canvas, proposed, mL, mT, minSize, grid);
  }
  const h = resizeAxis(
    region.x,
    region.x + region.w,
    mL,
    mR,
    proposed.x,
    proposed.x + proposed.w,
    canvas.w,
    minSize,
    grid,
  );
  const v = resizeAxis(
    region.y,
    region.y + region.h,
    mT,
    mB,
    proposed.y,
    proposed.y + proposed.h,
    canvas.h,
    minSize,
    grid,
  );
  return { x: h.pos, y: v.pos, w: h.size, h: v.size };
}

// One free axis. The anchor (the non-moving edge) stays fixed; size is
// snapped, floored at minSize, then capped so the moving edge can't leave the
// canvas. A center handle (neither edge moves) leaves the axis untouched.
function resizeAxis(
  lo: number,
  hi: number,
  movesLo: boolean,
  movesHi: boolean,
  propLo: number,
  propHi: number,
  max: number,
  minSize: number,
  grid: number,
): { pos: number; size: number } {
  if (!movesLo && !movesHi) return { pos: lo, size: hi - lo };
  let size = movesLo ? hi - propLo : propHi - lo;
  if (grid > 0) size = snapVal(size, grid);
  size = Math.max(minSize, size);
  size = Math.round(Math.min(size, movesLo ? hi : max - lo));
  return { pos: movesLo ? hi - size : lo, size };
}

// Floor a size pair at minSize while keeping the locked aspect ratio.
function floorArMin(
  nw: number,
  nh: number,
  ar: number,
  minSize: number,
): { nw: number; nh: number } {
  if (nw < minSize) {
    nw = minSize;
    nh = nw / ar;
  }
  if (nh < minSize) {
    nh = minSize;
    nw = nh * ar;
  }
  return { nw, nh };
}

// Shrink a rect so it fits the canvas without moving its top-left anchor: a
// sub-pixel overflow trims the far side, never relocates the box.
function shrinkIntoCanvas(
  x: number,
  y: number,
  w: number,
  h: number,
  canvas: Size,
): Rect {
  if (x < 0) {
    w += x;
    x = 0;
  }
  if (y < 0) {
    h += y;
    y = 0;
  }
  if (x + w > canvas.w) w = canvas.w - x;
  if (y + h > canvas.h) h = canvas.h - y;
  return { x, y, w, h };
}

// A corner: both axes move and aspect ratio is locked. The opposite corner is
// the fixed anchor; the box is reconciled to AR, snapped, floored, then capped
// by whichever axis binds against the canvas first.
function resizeCorner(
  region: Rect,
  canvas: Size,
  proposed: Rect,
  mL: boolean,
  mT: boolean,
  minSize: number,
  grid: number,
): Rect {
  const ar = region.w / region.h;
  const right = region.x + region.w;
  const bottom = region.y + region.h;

  let nw = mL ? right - proposed.x : proposed.x + proposed.w - region.x;
  let nh = mT ? bottom - proposed.y : proposed.y + proposed.h - region.y;
  if (nw / nh > ar) nh = nw / ar;
  else nw = nh * ar; // reconcile to AR (diagonal)
  if (grid > 0) {
    nw = snapVal(nw, grid);
    nh = nw / ar;
  } // width-driven grid snap
  ({ nw, nh } = floorArMin(nw, nh, ar, minSize));

  const maxW = mL ? right : canvas.w - region.x;
  const maxH = mT ? bottom : canvas.h - region.y;
  const cap = Math.min(maxW, maxH * ar);
  if (nw > cap) {
    nw = cap;
    nh = nw / ar;
  }

  nw = Math.round(nw);
  nh = Math.round(nh);
  const x = mL ? right - nw : region.x;
  const y = mT ? bottom - nh : region.y;
  return shrinkIntoCanvas(x, y, nw, nh, canvas);
}

/**
 * Content placement in REGION-LOCAL pixels (origin = region top-left).
 * - stretch: fills the region exactly, ignoring AR.
 * - fit: AR-correct, fully inside the region, centered (letterbox/pillarbox).
 * - cover: AR-correct, covers the region (overflow clipped), then zoom (>=1),
 *   then pan; the result always covers the region for any pan in [0,1].
 * Cover/fit use the rotation-adjusted source AR (effectiveAr).
 */
export function computeContentPlacement(
  region: Size,
  src: Size,
  c: ContentTransform,
): Rect {
  // Stretch, or degenerate dims (a source with no frame yet reports 0×0) → just
  // fill the region. Guards against NaN/Infinity from the AR divisions below.
  if (
    c.fill === "stretch" ||
    src.w <= 0 ||
    src.h <= 0 ||
    region.w <= 0 ||
    region.h <= 0
  ) {
    return { x: 0, y: 0, w: region.w, h: region.h };
  }

  const eAr = effectiveAr(src.w, src.h, c.rotation);
  const regionAr = region.w / region.h;

  if (c.fill === "fit") {
    if (eAr > regionAr) {
      const h = region.w / eAr;
      return { x: 0, y: (region.h - h) / 2, w: region.w, h };
    }
    const w = region.h * eAr;
    return { x: (region.w - w) / 2, y: 0, w, h: region.h };
  }

  // cover
  let fw: number;
  let fh: number;
  if (eAr > regionAr) {
    fh = region.h;
    fw = region.h * eAr;
  } else {
    fw = region.w;
    fh = region.w / eAr;
  }
  const z = Math.max(1, c.zoom);
  fw *= z;
  fh *= z;
  const px = clamp(c.panX, 0, 1);
  const py = clamp(c.panY, 0, 1);
  // `|| 0` normalizes -0 (from -(0)*px) to +0 so the value never serializes oddly.
  return {
    x: -(fw - region.w) * px || 0,
    y: -(fh - region.h) * py || 0,
    w: fw,
    h: fh,
  };
}

/**
 * Convert a region-local pixel drag of the content into a new normalized pan.
 * Dragging the content right reveals more of its left side → panX decreases
 * (matches the backend crop_x convention).
 */
export function panContentByPixels(args: {
  region: Size;
  src: Size;
  content: ContentTransform;
  dxPx: number;
  dyPx: number;
}): { panX: number; panY: number } {
  const { region, src, content, dxPx, dyPx } = args;
  const fp = computeContentPlacement(region, src, content);
  const exX = Math.max(1, fp.w - region.w);
  const exY = Math.max(1, fp.h - region.h);
  return {
    panX: clamp(content.panX - dxPx / exX, 0, 1),
    panY: clamp(content.panY - dyPx / exY, 0, 1),
  };
}

// ---------------------------------------------------------------------------
// Content ⇄ backend (wire LayoutSlot) mapping
// ---------------------------------------------------------------------------

const round2 = (v: number) => Math.round(v * 100) / 100;

/** Snap an arbitrary degree value to the nearest of 0/90/180/270. */
export function nearestRotation(deg: number): Rotation {
  if (!Number.isFinite(deg)) return 0;
  const r = (((Math.round(deg / 90) * 90) % 360) + 360) % 360;
  return r as Rotation;
}

/**
 * Editor Content → the wire LayoutSlot fields. The UI 'cover' maps to the
 * frozen wire value 'crop'; fit/stretch ignore crop (backend centers them).
 */
export function contentToBackend(c: ContentTransform): {
  rotation: number;
  aspect_ratio_mode: AspectRatioMode;
  crop?: CropConfig;
} {
  if (c.fill === "cover") {
    return {
      rotation: c.rotation,
      aspect_ratio_mode: "crop",
      crop: {
        x: round2(clamp(c.panX, 0, 1)),
        y: round2(clamp(c.panY, 0, 1)),
        scale: round2(Math.max(1, c.zoom)),
      },
    };
  }
  return { rotation: c.rotation, aspect_ratio_mode: c.fill };
}

/** Wire LayoutSlot → editor Content. Unknown/undefined mode = stretch (backend default). */
export function backendToContent(
  slot: Pick<LayoutSlot, "rotation" | "aspect_ratio_mode" | "crop">,
): ContentTransform {
  const rotation = nearestRotation(slot.rotation ?? 0);
  if (slot.aspect_ratio_mode === "crop") {
    return {
      rotation,
      fill: "cover",
      panX: slot.crop?.x ?? 0.5,
      panY: slot.crop?.y ?? 0.5,
      zoom: Math.max(1, slot.crop?.scale ?? 1),
    };
  }
  const fill: FillMode = slot.aspect_ratio_mode === "fit" ? "fit" : "stretch";
  return { rotation, fill, panX: 0.5, panY: 0.5, zoom: 1 };
}

/**
 * Merge a content transform onto a wire slot: sets rotation/aspect_ratio_mode/crop,
 * dropping crop when the fill isn't cover. The single source of this mapping —
 * used by both the inspector and the store's commitContent so they can't drift.
 */
export function applyContentToSlot(
  slot: LayoutSlot,
  content: ContentTransform,
): LayoutSlot {
  const back = contentToBackend(content);
  const next: LayoutSlot = {
    ...slot,
    rotation: back.rotation,
    aspect_ratio_mode: back.aspect_ratio_mode,
  };
  if (back.crop) next.crop = back.crop;
  else delete next.crop;
  return next;
}

// ---------------------------------------------------------------------------
// Alignment-guide candidates (region snapping)
// ---------------------------------------------------------------------------

export interface RegionRef extends Rect {
  input: string;
}

/**
 * Snap-line candidates for a dragged region: canvas left/center/right (and
 * top/center/bottom) plus every OTHER region's edges and centers. The dragged
 * region is excluded. Feed each axis into findAlignmentSnap.
 */
export function collectAlignmentCandidates(
  regions: RegionRef[],
  draggedInput: string,
  canvas: Size,
): { vertical: number[]; horizontal: number[] } {
  const vertical = [0, canvas.w / 2, canvas.w];
  const horizontal = [0, canvas.h / 2, canvas.h];
  for (const r of regions) {
    if (r.input === draggedInput) continue;
    vertical.push(r.x, r.x + r.w / 2, r.x + r.w);
    horizontal.push(r.y, r.y + r.h / 2, r.y + r.h);
  }
  return { vertical, horizontal };
}
