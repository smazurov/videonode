// Canvas mask geometry — which canvas pixels the compositor actually paints.
//
// The authority for this math is composer/src/render/build_source_slot.cpp:82-98.
// A slot's on-canvas footprint is one axis-aligned rect: rotation is applied to
// the SOURCE frame (PL_ROTATION_*), and crop only re-windows the source, so
// neither moves the footprint. Only `fit` shrinks it, leaving letterbox bars that
// are canvas background rather than video.
//
// Framework-free: no React, no Konva.

import type { Source } from "../../hooks/slices/types";
import type { LayoutSlot } from "../../lib/composer-types";
import {
  backendToContent,
  computeContentPlacement,
  type Rect,
  type Size,
} from "./region-content";

/**
 * Canvas-absolute footprint of one layout slot. `src` is the real source size;
 * pass the region's own size when it is unknown, which degenerates fit/cover to
 * filling the region (matching build_source_slot.cpp's own `frame.width > 0` guard).
 */
export function slotFootprint(slot: LayoutSlot, src: Size): Rect {
  const region = { w: slot.w, h: slot.h };
  const fp = computeContentPlacement(region, src, backendToContent(slot));
  // Content is clipped to its region, so cover's overflow never reaches the canvas.
  const x0 = slot.x + Math.max(0, fp.x);
  const y0 = slot.y + Math.max(0, fp.y);
  const x1 = slot.x + Math.min(slot.w, fp.x + fp.w);
  const y1 = slot.y + Math.min(slot.h, fp.y + fp.h);
  return { x: x0, y: y0, w: Math.max(0, x1 - x0), h: Math.max(0, y1 - y0) };
}

function clampToCanvas(r: Rect, canvas: Size): Rect | null {
  const x0 = Math.max(0, r.x);
  const y0 = Math.max(0, r.y);
  const x1 = Math.min(canvas.w, r.x + r.w);
  const y1 = Math.min(canvas.h, r.y + r.h);
  if (x1 <= x0 || y1 <= y0) return null;
  return { x: x0, y: y0, w: x1 - x0, h: y1 - y0 };
}

/**
 * Every slot's footprint, clipped to the canvas. Slots may sit partly or wholly
 * off-canvas — the control proto allows negative x/y.
 */
export function layoutFootprints(
  layout: readonly LayoutSlot[],
  canvas: Size,
  sourceDims?: ReadonlyMap<string, Size>,
): Rect[] {
  const out: Rect[] = [];
  for (const slot of layout) {
    const src = sourceDims?.get(slot.input) ?? { w: slot.w, h: slot.h };
    const clipped = clampToCanvas(slotFootprint(slot, src), canvas);
    if (clipped) out.push(clipped);
  }
  return out;
}

const coord = (v: number) => String(Math.round(v * 1e6) / 1e6);

/**
 * Path data for an SVG `<clipPath clipPathUnits="objectBoundingBox">`: one
 * subpath per rect, normalized to 0..1. objectBoundingBox units are fractions of
 * the clipped element's box, so the same path scales to any player size.
 */
export function clipPathData(rects: readonly Rect[], canvas: Size): string {
  if (canvas.w <= 0 || canvas.h <= 0) return "";
  return rects
    .map((r) => {
      const x0 = coord(r.x / canvas.w);
      const y0 = coord(r.y / canvas.h);
      const x1 = coord((r.x + r.w) / canvas.w);
      const y1 = coord((r.y + r.h) / canvas.h);
      return `M${x0} ${y0}H${x1}V${y1}H${x0}Z`;
    })
    .join(" ");
}

/** Rects as "x,y,w,h;x,y,w,h" for the /video URL. */
export function encodeClip(rects: readonly Rect[]): string {
  return rects
    .map((r) => [r.x, r.y, r.w, r.h].map((v) => Math.round(v)).join(","))
    .join(";");
}

/** Inverse of encodeClip. Malformed or empty rects are dropped, never thrown. */
export function parseClip(value: string | null | undefined): Rect[] {
  if (!value) return [];
  const out: Rect[] = [];
  for (const part of value.split(";")) {
    if (!part) continue;
    const fields = part.split(",");
    if (fields.length !== 4) continue;
    const [x, y, w, h] = fields.map(Number) as [number, number, number, number];
    if (![x, y, w, h].every((v) => Number.isFinite(v))) continue;
    if (w <= 0 || h <= 0) continue;
    out.push({ x, y, w, h });
  }
  return out;
}

/** Parse a "WxH" canvas size for the /video URL. */
export function parseCanvasSize(value: string | null | undefined): Size | null {
  if (!value) return null;
  const m = /^(\d+)x(\d+)$/.exec(value.trim());
  if (!m) return null;
  const w = Number(m[1]);
  const h = Number(m[2]);
  return w > 0 && h > 0 ? { w, h } : null;
}

/** Format a canvas size for the /video URL. */
export function encodeCanvasSize(canvas: Size): string {
  return `${Math.round(canvas.w)}x${Math.round(canvas.h)}`;
}

/**
 * Real source dimensions per composer input ref, preferring the live negotiated
 * format over the configured one. Refs whose size is unknown are omitted, so
 * callers fall back to the region's own size.
 */
export function buildSourceDims(
  inputs: readonly { readonly ref: string }[],
  sourcesById: Readonly<Record<string, Source>>,
): Map<string, Size> {
  const dims = new Map<string, Size>();
  for (const input of inputs) {
    const src = sourcesById[input.ref.replace(/^source:/, "")];
    const w = src?.latest_status?.format?.w ?? src?.format?.width;
    const h = src?.latest_status?.format?.h ?? src?.format?.height;
    if (w && h) dims.set(input.ref, { w, h });
  }
  return dims;
}
