// Resize-handle names recur across many cases — constants would hurt readability here.
/* eslint-disable sonarjs/no-duplicate-string */
import Konva from "konva";
import { describe, expect, it, test } from "vitest";

import {
  type ContentTransform,
  type Rect,
  type ResizeHandle,
  type Rotation,
  type Size,
  type RegionRef,
  backendToContent,
  clampRegionMove,
  collectAlignmentCandidates,
  computeContentPlacement,
  contentToBackend,
  effectiveAr,
  nearestRotation,
  panContentByPixels,
  resizeRegion,
} from "./region-content";

// --- helpers ---------------------------------------------------------------

function rand(lo: number, hi: number) {
  // eslint-disable-next-line sonarjs/pseudo-random
  return lo + Math.random() * (hi - lo);
}
function randInt(lo: number, hi: number) {
  return Math.floor(rand(lo, hi));
}

const FUZZ = 10_000;
const ITER = "iteration %i";
const CANVAS = { w: 1920, h: 1080 };

const ALL_HANDLES: ResizeHandle[] = [
  "top-left",
  "top-center",
  "top-right",
  "middle-left",
  "middle-right",
  "bottom-left",
  "bottom-center",
  "bottom-right",
];
const isLeft = (h: ResizeHandle) => h.endsWith("-left");
const isRight = (h: ResizeHandle) => h.endsWith("-right");
const isTop = (h: ResizeHandle) => h.startsWith("top-");
const isBottom = (h: ResizeHandle) => h.startsWith("bottom-");
const isCorner = (h: ResizeHandle) =>
  (isLeft(h) || isRight(h)) && (isTop(h) || isBottom(h));

// =====================================================================
// effectiveAr
// =====================================================================

describe("effectiveAr", () => {
  it("unchanged for 0 and 180", () => {
    expect(effectiveAr(1920, 1080, 0)).toBeCloseTo(1920 / 1080, 6);
    expect(effectiveAr(1920, 1080, 180)).toBeCloseTo(1920 / 1080, 6);
  });
  it("inverted for 90 and 270", () => {
    expect(effectiveAr(1920, 1080, 90)).toBeCloseTo(1080 / 1920, 6);
    expect(effectiveAr(1920, 1080, 270)).toBeCloseTo(1080 / 1920, 6);
  });
});

// =====================================================================
// Group A — region move clamp ("region ⊆ canvas, always")
// =====================================================================

describe("clampRegionMove", () => {
  const C = CANVAS;
  it("A1 in-bounds passthrough", () => {
    expect(
      clampRegionMove({ x: 100, y: 100, w: 400, h: 300 }, C, false, 0),
    ).toEqual({ x: 100, y: 100, w: 400, h: 300 });
  });
  it("A2 clamp left", () => {
    expect(
      clampRegionMove({ x: -50, y: 100, w: 400, h: 300 }, C, false, 0),
    ).toEqual({ x: 0, y: 100, w: 400, h: 300 });
  });
  it("A3 clamp top", () => {
    expect(
      clampRegionMove({ x: 100, y: -50, w: 400, h: 300 }, C, false, 0).y,
    ).toBe(0);
  });
  it("A4 clamp right edge", () => {
    expect(
      clampRegionMove({ x: 1700, y: 100, w: 400, h: 300 }, C, false, 0).x,
    ).toBe(1520);
  });
  it("A5 clamp bottom edge", () => {
    expect(
      clampRegionMove({ x: 100, y: 900, w: 400, h: 300 }, C, false, 0).y,
    ).toBe(780);
  });
  it("A6 clamp both corners (bottom-right)", () => {
    expect(
      clampRegionMove({ x: 5000, y: 5000, w: 400, h: 300 }, C, false, 0),
    ).toEqual({ x: 1520, y: 780, w: 400, h: 300 });
  });
  it("A7 clamp top-left corner", () => {
    expect(
      clampRegionMove({ x: -9999, y: -9999, w: 400, h: 300 }, C, false, 0),
    ).toEqual({ x: 0, y: 0, w: 400, h: 300 });
  });
  it("A8 region exactly canvas size, nudge pins to 0", () => {
    expect(
      clampRegionMove({ x: 10, y: 10, w: 1920, h: 1080 }, C, false, 0),
    ).toEqual({ x: 0, y: 0, w: 1920, h: 1080 });
  });
  it("A9 region wider than canvas pins x to 0, size unchanged", () => {
    expect(
      clampRegionMove({ x: 50, y: 0, w: 3000, h: 300 }, C, false, 0),
    ).toEqual({ x: 0, y: 0, w: 3000, h: 300 });
  });
  it("grid snaps before clamping", () => {
    expect(
      clampRegionMove({ x: 103, y: 97, w: 400, h: 300 }, C, true, 10),
    ).toEqual({ x: 100, y: 100, w: 400, h: 300 });
  });
});

describe("fuzz: clampRegionMove keeps region ⊆ canvas, size unchanged", () => {
  test.each(Array.from({ length: FUZZ }, (_, i) => i))(ITER, () => {
    const region: Rect = {
      x: rand(-3000, 3000),
      y: rand(-3000, 3000),
      w: randInt(16, 1920),
      h: randInt(16, 1080),
    };
    const out = clampRegionMove(region, CANVAS, false, 0);
    expect(out.w).toBe(region.w);
    expect(out.h).toBe(region.h);
    expect(out.x).toBeGreaterThanOrEqual(0);
    expect(out.y).toBeGreaterThanOrEqual(0);
    // when region fits, it stays fully inside
    if (region.w <= CANVAS.w)
      expect(out.x + out.w).toBeLessThanOrEqual(CANVAS.w + 1e-9);
    if (region.h <= CANVAS.h)
      expect(out.y + out.h).toBeLessThanOrEqual(CANVAS.h + 1e-9);
  });
});

// =====================================================================
// Group B — region resize edge-cap (the headline regression)
// =====================================================================

describe("resizeRegion — free edges (single axis)", () => {
  const C = CANVAS;
  const base: Rect = { x: 100, y: 100, w: 400, h: 300 };

  it("B1 right edge grows within bounds, x/y fixed", () => {
    expect(
      resizeRegion({
        region: base,
        handle: "middle-right",
        canvas: C,
        proposed: { x: 100, y: 100, w: 600, h: 300 },
        lockAspect: false,
      }),
    ).toEqual({ x: 100, y: 100, w: 600, h: 300 });
  });

  it("B2 right edge past canvas EDGE-CAPS: x unchanged, w capped", () => {
    const r = resizeRegion({
      region: { x: 1400, y: 100, w: 400, h: 300 },
      handle: "middle-right",
      canvas: C,
      proposed: { x: 1400, y: 100, w: 900, h: 300 },
      lockAspect: false,
    });
    expect(r.x).toBe(1400); // anchor NOT moved
    expect(r.w).toBe(520); // capped at 1920-1400
    expect(r.y).toBe(100);
    expect(r.h).toBe(300);
  });

  it("B3 left edge drag right, right edge anchored", () => {
    expect(
      resizeRegion({
        region: base,
        handle: "middle-left",
        canvas: C,
        proposed: { x: 250, y: 100, w: 250, h: 300 },
        lockAspect: false,
      }),
    ).toEqual({ x: 250, y: 100, w: 250, h: 300 });
  });

  it("B4 left edge past 0 edge-caps: right edge stays at 500", () => {
    const r = resizeRegion({
      region: base,
      handle: "middle-left",
      canvas: C,
      proposed: { x: -200, y: 100, w: 700, h: 300 },
      lockAspect: false,
    });
    expect(r.x).toBe(0);
    expect(r.x + r.w).toBe(500); // anchored right edge fixed
  });

  it("B5 bottom edge past bottom caps", () => {
    const r = resizeRegion({
      region: { x: 100, y: 900, w: 400, h: 300 },
      handle: "bottom-center",
      canvas: C,
      proposed: { x: 100, y: 900, w: 400, h: 700 },
      lockAspect: false,
    });
    expect(r.y).toBe(900);
    expect(r.h).toBe(180);
  });

  it("B6 top edge past 0 caps, bottom anchored at 400", () => {
    const r = resizeRegion({
      region: base,
      handle: "top-center",
      canvas: C,
      proposed: { x: 100, y: -300, w: 400, h: 700 },
      lockAspect: false,
    });
    expect(r.y).toBe(0);
    expect(r.y + r.h).toBe(400);
  });

  it("B7 min-size floor on right edge, x fixed", () => {
    const r = resizeRegion({
      region: base,
      handle: "middle-right",
      canvas: C,
      proposed: { x: 100, y: 100, w: 10, h: 300 },
      lockAspect: false,
    });
    expect(r.w).toBe(16);
    expect(r.x).toBe(100);
  });

  it("B8 min-size on left-anchor keeps right edge at 500", () => {
    const r = resizeRegion({
      region: base,
      handle: "middle-left",
      canvas: C,
      proposed: { x: 490, y: 100, w: 10, h: 300 },
      lockAspect: false,
    });
    expect(r.w).toBe(16);
    expect(r.x + r.w).toBe(500);
    expect(r.x).toBe(484);
  });
});

describe("resizeRegion — locked corners (aspect ratio)", () => {
  const C = CANVAS;
  it("B9 bottom-right grows, AR 4:3 kept, top-left fixed", () => {
    const r = resizeRegion({
      region: { x: 100, y: 100, w: 400, h: 300 },
      handle: "bottom-right",
      canvas: C,
      proposed: { x: 100, y: 100, w: 600, h: 450 },
      lockAspect: true,
    });
    expect(r.x).toBe(100);
    expect(r.y).toBe(100);
    expect(r.w).toBe(600);
    expect(r.h).toBe(450);
    expect(r.w / r.h).toBeCloseTo(4 / 3, 2);
  });

  it("B10 bottom-right past right edge: AR kept, capped, top-left fixed", () => {
    const r = resizeRegion({
      region: { x: 1400, y: 100, w: 400, h: 300 },
      handle: "bottom-right",
      canvas: C,
      proposed: { x: 1400, y: 100, w: 2000, h: 1500 },
      lockAspect: true,
    });
    expect(r.x).toBe(1400);
    expect(r.y).toBe(100);
    expect(r.w).toBe(520); // capped at 1920-1400
    expect(r.w / r.h).toBeCloseTo(4 / 3, 2);
    expect(r.x + r.w).toBeLessThanOrEqual(1920);
    expect(r.y + r.h).toBeLessThanOrEqual(1080);
  });

  it("B11 bottom-right where height binds (AR + cap)", () => {
    const r = resizeRegion({
      region: { x: 100, y: 800, w: 400, h: 300 },
      handle: "bottom-right",
      canvas: C,
      proposed: { x: 100, y: 800, w: 2000, h: 2000 },
      lockAspect: true,
    });
    expect(r.x).toBe(100);
    expect(r.y).toBe(800);
    expect(r.h).toBe(280); // capped at 1080-800
    expect(r.w / r.h).toBeCloseTo(4 / 3, 2);
    expect(r.y + r.h).toBeLessThanOrEqual(1080);
  });

  it("B12 top-left corner drag, bottom-right anchored, AR kept", () => {
    const r = resizeRegion({
      region: { x: 200, y: 200, w: 400, h: 300 },
      handle: "top-left",
      canvas: C,
      proposed: { x: 80, y: 110, w: 520, h: 390 },
      lockAspect: true,
    });
    expect(r.x + r.w).toBe(600); // anchored right
    expect(r.y + r.h).toBe(500); // anchored bottom
    expect(r.w / r.h).toBeCloseTo(4 / 3, 2);
  });
});

describe("resizeRegion — grid snap ordering", () => {
  const C = CANVAS;
  it("B14 snap applied before clamp; stays in bounds and grid-aligned", () => {
    const r = resizeRegion({
      region: { x: 1400, y: 100, w: 400, h: 300 },
      handle: "middle-right",
      canvas: C,
      proposed: { x: 1400, y: 100, w: 518, h: 300 },
      lockAspect: false,
      doSnap: true,
      grid: 10,
    });
    expect(r.w % 10).toBe(0);
    expect(r.x + r.w).toBeLessThanOrEqual(1920);
  });
  it("B15 clamp wins over grid at the hard edge", () => {
    const r = resizeRegion({
      region: { x: 1415, y: 100, w: 400, h: 300 },
      handle: "middle-right",
      canvas: C,
      proposed: { x: 1415, y: 100, w: 520, h: 300 },
      lockAspect: false,
      doSnap: true,
      grid: 10,
    });
    expect(r.x + r.w).toBe(1920); // bounds beat grid alignment
  });
});

describe("fuzz: resizeRegion keeps anchor fixed, stays ⊆ canvas, min-size, AR on corners", () => {
  test.each(Array.from({ length: FUZZ }, (_, i) => i))(ITER, () => {
    // region fully inside the canvas
    const w = randInt(40, 800),
      h = randInt(40, 600);
    const region: Rect = {
      x: randInt(0, CANVAS.w - w),
      y: randInt(0, CANVAS.h - h),
      w,
      h,
    };
    const handle = ALL_HANDLES[randInt(0, ALL_HANDLES.length)]!;
    const lockAspect = isCorner(handle);
    // a wild proposed rect (gesture could be anything)
    const proposed: Rect = {
      x: region.x + rand(-500, 200),
      y: region.y + rand(-500, 200),
      w: rand(20, 2500),
      h: rand(20, 2000),
    };
    const out = resizeRegion({
      region,
      handle,
      canvas: CANVAS,
      proposed,
      lockAspect,
    });

    // anchored edges bit-identical
    if (isRight(handle)) expect(out.x).toBe(region.x);
    if (isLeft(handle)) expect(out.x + out.w).toBe(region.x + region.w);
    if (isBottom(handle)) expect(out.y).toBe(region.y);
    if (isTop(handle)) expect(out.y + out.h).toBe(region.y + region.h);
    if (!isLeft(handle) && !isRight(handle)) {
      expect(out.x).toBe(region.x);
      expect(out.w).toBe(region.w);
    }
    if (!isTop(handle) && !isBottom(handle)) {
      expect(out.y).toBe(region.y);
      expect(out.h).toBe(region.h);
    }
    // ⊆ canvas
    expect(out.x).toBeGreaterThanOrEqual(0);
    expect(out.y).toBeGreaterThanOrEqual(0);
    expect(out.x + out.w).toBeLessThanOrEqual(CANVAS.w);
    expect(out.y + out.h).toBeLessThanOrEqual(CANVAS.h);
    // min size (unless the available space is itself below min — regions here always fit)
    expect(out.w).toBeGreaterThanOrEqual(16);
    expect(out.h).toBeGreaterThanOrEqual(16);
    // AR preserved on corners — except where bounds or min-size legitimately win
    // (you can't hold a ratio when the box is pinned to an edge or floored to min).
    // Tolerance is rounding-aware: integer pixels perturb the ratio by ~ar·(1/w+1/h).
    if (lockAspect) {
      const ar = region.w / region.h;
      const minBound = out.w <= 16 + 1e-9 || out.h <= 16 + 1e-9;
      const edgeBound =
        out.x <= 1e-9 ||
        out.y <= 1e-9 ||
        out.x + out.w >= CANVAS.w - 1e-9 ||
        out.y + out.h >= CANVAS.h - 1e-9;
      if (!minBound && !edgeBound) {
        const tol = 2 * ar * (1 / out.w + 1 / out.h) + 1e-9;
        expect(Math.abs(out.w / out.h - ar)).toBeLessThanOrEqual(tol);
      }
    }
  });
});

// =====================================================================
// Konva oracle — region rect AABB matches (rotation 0, always)
// =====================================================================

describe("konva oracle: region is axis-aligned", () => {
  it("region rect client AABB equals its x/y/w/h", () => {
    // NB: never create a Konva.Layer/Stage in unit tests — those instantiate a
    // canvas and throw headless. A parent Group is a canvas-free frame of reference.
    const region = { x: 300, y: 150, w: 200, h: 120 };
    const canvasFrame = new Konva.Group({ x: 0, y: 0 });
    const g = new Konva.Group({ x: region.x, y: region.y, rotation: 0 });
    const r = new Konva.Rect({ width: region.w, height: region.h });
    g.add(r);
    canvasFrame.add(g);
    const box = r.getClientRect({ relativeTo: canvasFrame, skipStroke: true });
    expect(box).toMatchObject({ x: 300, y: 150, width: 200, height: 120 });
  });
});

// =====================================================================
// Group C — content fill (cover / fit / stretch), rotation 0
// =====================================================================

const COVER = (
  rotation: Rotation = 0,
  panX = 0.5,
  panY = 0.5,
  zoom = 1,
): ContentTransform => ({ rotation, fill: "cover", panX, panY, zoom });
const FIT = (rotation: Rotation = 0): ContentTransform => ({
  rotation,
  fill: "fit",
  panX: 0.5,
  panY: 0.5,
  zoom: 1,
});
const STRETCH: ContentTransform = {
  rotation: 0,
  fill: "stretch",
  panX: 0.5,
  panY: 0.5,
  zoom: 1,
};

function covers(fp: Rect, region: Size, eps = 0.01) {
  return (
    fp.x <= eps &&
    fp.y <= eps &&
    fp.x + fp.w >= region.w - eps &&
    fp.y + fp.h >= region.h - eps
  );
}

describe("computeContentPlacement — cover", () => {
  it("C1 landscape src in portrait region", () => {
    const fp = computeContentPlacement(
      { w: 100, h: 200 },
      { w: 400, h: 100 },
      COVER(),
    );
    expect(fp).toMatchObject({ x: -350, y: 0, w: 800, h: 200 });
    expect(covers(fp, { w: 100, h: 200 })).toBe(true);
  });
  it("C2 portrait src in landscape region", () => {
    const fp = computeContentPlacement(
      { w: 200, h: 100 },
      { w: 100, h: 400 },
      COVER(),
    );
    expect(fp).toMatchObject({ x: 0, y: -350, w: 200, h: 800 });
  });
  it("C4 matching AR fills exactly", () => {
    const fp = computeContentPlacement(
      { w: 960, h: 540 },
      { w: 1920, h: 1080 },
      COVER(),
    );
    expect(fp.x).toBeCloseTo(0, 6);
    expect(fp.y).toBeCloseTo(0, 6);
    expect(fp.w).toBeCloseTo(960, 6);
    expect(fp.h).toBeCloseTo(540, 6);
  });
  it("C5 cover touches exactly two opposite edges", () => {
    const fp = computeContentPlacement(
      { w: 100, h: 200 },
      { w: 400, h: 100 },
      COVER(),
    );
    expect(fp.y).toBeCloseTo(0);
    expect(fp.y + fp.h).toBeCloseTo(200); // top+bottom flush
    expect(fp.x).toBeLessThan(0);
    expect(fp.x + fp.w).toBeGreaterThan(100); // L/R overflow
  });
});

describe("computeContentPlacement — fit", () => {
  it("C6 pillarbox when source taller", () => {
    const fp = computeContentPlacement(
      { w: 960, h: 540 },
      { w: 1080, h: 1920 },
      FIT(),
    );
    expect(fp.w).toBeCloseTo(303.75);
    expect(fp.h).toBe(540);
    expect(fp.x).toBeCloseTo(328.125);
    expect(fp.y).toBe(0);
  });
  it("C7 letterbox when source wider", () => {
    const fp = computeContentPlacement(
      { w: 540, h: 540 },
      { w: 1920, h: 1080 },
      FIT(),
    );
    expect(fp.w).toBe(540);
    expect(fp.h).toBeCloseTo(303.75);
    expect(fp.x).toBe(0);
    expect(fp.y).toBeCloseTo(118.125);
  });
  it("C8 fit stays inside region", () => {
    const fp = computeContentPlacement(
      { w: 960, h: 540 },
      { w: 1080, h: 1920 },
      FIT(),
    );
    expect(fp.x).toBeGreaterThanOrEqual(-1e-9);
    expect(fp.y).toBeGreaterThanOrEqual(-1e-9);
    expect(fp.x + fp.w).toBeLessThanOrEqual(960 + 1e-9);
    expect(fp.y + fp.h).toBeLessThanOrEqual(540 + 1e-9);
  });
});

describe("computeContentPlacement — stretch", () => {
  it("C9 fills exactly, C10 ignores pan/zoom", () => {
    expect(
      computeContentPlacement({ w: 960, h: 540 }, { w: 640, h: 480 }, STRETCH),
    ).toEqual({ x: 0, y: 0, w: 960, h: 540 });
    expect(
      computeContentPlacement(
        { w: 960, h: 540 },
        { w: 640, h: 480 },
        { rotation: 0, fill: "stretch", panX: 0.2, panY: 0.8, zoom: 2 },
      ),
    ).toEqual({ x: 0, y: 0, w: 960, h: 540 });
  });
});

// =====================================================================
// Group D — content rotation (effective AR; region unaffected)
// =====================================================================

describe("computeContentPlacement — rotation", () => {
  it("D5 fit with rotation 90 uses inverted AR (pillarbox)", () => {
    // 1920x1080 rotated 90 → effective 9:16; pillarbox in 960x540
    const fp = computeContentPlacement(
      { w: 960, h: 540 },
      { w: 1920, h: 1080 },
      FIT(90),
    );
    expect(fp.w).toBeCloseTo(303.75);
    expect(fp.h).toBe(540);
  });
  it("D7 cover 0 ≡ 180 footprint", () => {
    const a = computeContentPlacement(
      { w: 960, h: 540 },
      { w: 1920, h: 1080 },
      COVER(0),
    );
    const b = computeContentPlacement(
      { w: 960, h: 540 },
      { w: 1920, h: 1080 },
      COVER(180),
    );
    expect(b).toEqual(a);
  });
  it("D8 cover 90 ≡ 270 footprint", () => {
    const a = computeContentPlacement(
      { w: 960, h: 540 },
      { w: 1920, h: 1080 },
      COVER(90),
    );
    const b = computeContentPlacement(
      { w: 960, h: 540 },
      { w: 1920, h: 1080 },
      COVER(270),
    );
    expect(b).toEqual(a);
  });
  it("D-cover rot90 still covers region", () => {
    const fp = computeContentPlacement(
      { w: 960, h: 540 },
      { w: 1920, h: 1080 },
      COVER(90),
    );
    expect(covers(fp, { w: 960, h: 540 })).toBe(true);
  });
});

// =====================================================================
// Group E — pan & zoom (cover coverage; content not canvas-clamped)
// =====================================================================

describe("content pan & zoom", () => {
  const R = { w: 960, h: 540 };
  const S = { w: 640, h: 480 };
  it("E1 center pan default", () => {
    expect(computeContentPlacement(R, S, COVER(0, 0.5, 0.5))).toMatchObject({
      x: 0,
      y: -90,
      w: 960,
      h: 720,
    });
  });
  it("E2 pan top", () => {
    expect(computeContentPlacement(R, S, COVER(0, 0.5, 0)).y).toBeCloseTo(0);
  });
  it("E3 pan bottom", () => {
    expect(computeContentPlacement(R, S, COVER(0, 0.5, 1)).y).toBeCloseTo(-180);
  });
  it("E6 zoom 2 still covers", () => {
    const fp = computeContentPlacement(R, S, COVER(0, 0.5, 0.5, 2));
    expect(fp.w).toBeGreaterThanOrEqual(R.w);
    expect(fp.h).toBeGreaterThanOrEqual(R.h);
    expect(covers(fp, R)).toBe(true);
  });
  it("E7 zoom < 1 is floored to 1 by placement", () => {
    expect(computeContentPlacement(R, S, COVER(0, 0.5, 0.5, 0.5))).toEqual(
      computeContentPlacement(R, S, COVER(0, 0.5, 0.5, 1)),
    );
  });
  it("placement guards degenerate source dims (0x0) → fills region", () => {
    expect(computeContentPlacement(R, { w: 0, h: 0 }, COVER())).toEqual({
      x: 0,
      y: 0,
      w: R.w,
      h: R.h,
    });
  });
  it("panContentByPixels: drag right reveals left (panX decreases)", () => {
    const c = COVER(0, 0.5, 0.5);
    const out = panContentByPixels({
      region: R,
      src: S,
      content: c,
      dxPx: 50,
      dyPx: 0,
    });
    expect(out.panX).toBeLessThan(0.5);
    expect(out.panX).toBeGreaterThanOrEqual(0);
  });
  it("nearestRotation sanitizes non-finite input to 0", () => {
    expect(nearestRotation(Number.NaN)).toBe(0);
    expect(nearestRotation(Number.POSITIVE_INFINITY)).toBe(0);
  });
});

describe("fuzz: cover always covers the region (all rotations, pan, zoom)", () => {
  test.each(Array.from({ length: FUZZ }, (_, i) => i))(ITER, () => {
    const region = { w: randInt(16, 1920), h: randInt(16, 1080) };
    const src = { w: randInt(16, 4000), h: randInt(16, 4000) };
    const rotation = ([0, 90, 180, 270] as const)[randInt(0, 4)]!;
    const c: ContentTransform = {
      rotation,
      fill: "cover",
      panX: rand(0, 1),
      panY: rand(0, 1),
      zoom: rand(0.5, 3),
    };
    const fp = computeContentPlacement(region, src, c);
    expect(fp.x).toBeLessThanOrEqual(0.01);
    expect(fp.y).toBeLessThanOrEqual(0.01);
    expect(fp.x + fp.w).toBeGreaterThanOrEqual(region.w - 0.01);
    expect(fp.y + fp.h).toBeGreaterThanOrEqual(region.h - 0.01);
    // footprint AR equals the rotation-adjusted source AR
    expect(fp.w / fp.h).toBeCloseTo(effectiveAr(src.w, src.h, rotation), 3);
  });
});

// =====================================================================
// Konva oracle — rotating the source node yields the predicted footprint AABB
// =====================================================================

describe("konva oracle: content rotation produces the predicted footprint", () => {
  function clientFootprint(fp: Rect, rotation: Rotation): Rect {
    const frame = new Konva.Group({ x: 0, y: 0 });
    const dW = rotation % 180 === 0 ? fp.w : fp.h;
    const dH = rotation % 180 === 0 ? fp.h : fp.w;
    const node = new Konva.Rect({
      x: fp.x + fp.w / 2,
      y: fp.y + fp.h / 2,
      offsetX: dW / 2,
      offsetY: dH / 2,
      width: dW,
      height: dH,
      rotation,
    });
    frame.add(node);
    const b = node.getClientRect({ relativeTo: frame, skipStroke: true });
    return { x: b.x, y: b.y, w: b.width, h: b.height };
  }
  for (const rot of [0, 90, 180, 270] as const) {
    it(`rotation ${rot}: AABB matches computeContentPlacement`, () => {
      const region = { w: 960, h: 540 };
      const fp = computeContentPlacement(
        region,
        { w: 1920, h: 1080 },
        COVER(rot),
      );
      const got = clientFootprint(fp, rot);
      expect(got.x).toBeCloseTo(fp.x, 4);
      expect(got.y).toBeCloseTo(fp.y, 4);
      expect(got.w).toBeCloseTo(fp.w, 4);
      expect(got.h).toBeCloseTo(fp.h, 4);
    });
  }
});

// =====================================================================
// Group F — content ⇄ backend mapping
// =====================================================================

describe("nearestRotation", () => {
  it("snaps to 0/90/180/270", () => {
    expect(nearestRotation(0)).toBe(0);
    expect(nearestRotation(89)).toBe(90);
    expect(nearestRotation(200)).toBe(180);
    expect(nearestRotation(-90)).toBe(270);
    expect(nearestRotation(360)).toBe(0);
  });
});

describe("contentToBackend", () => {
  it("cover → crop with rounded pan/zoom", () => {
    expect(
      contentToBackend({
        rotation: 90,
        fill: "cover",
        panX: 0.2,
        panY: 0.8,
        zoom: 1.75,
      }),
    ).toEqual({
      rotation: 90,
      aspect_ratio_mode: "crop",
      crop: { x: 0.2, y: 0.8, scale: 1.75 },
    });
  });
  it("default cover → crop 0.5/0.5/1", () => {
    expect(
      contentToBackend({
        rotation: 0,
        fill: "cover",
        panX: 0.5,
        panY: 0.5,
        zoom: 1,
      }),
    ).toEqual({
      rotation: 0,
      aspect_ratio_mode: "crop",
      crop: { x: 0.5, y: 0.5, scale: 1 },
    });
  });
  it("fit/stretch omit crop", () => {
    expect(
      contentToBackend({
        rotation: 0,
        fill: "fit",
        panX: 0.5,
        panY: 0.5,
        zoom: 1,
      }),
    ).toEqual({ rotation: 0, aspect_ratio_mode: "fit" });
    expect(
      contentToBackend({
        rotation: 180,
        fill: "stretch",
        panX: 0.5,
        panY: 0.5,
        zoom: 1,
      }),
    ).toEqual({ rotation: 180, aspect_ratio_mode: "stretch" });
  });
});

describe("backendToContent", () => {
  it("crop → cover", () => {
    expect(
      backendToContent({
        rotation: 90,
        aspect_ratio_mode: "crop",
        crop: { x: 0.2, y: 0.8, scale: 1.5 },
      }),
    ).toEqual({ rotation: 90, fill: "cover", panX: 0.2, panY: 0.8, zoom: 1.5 });
  });
  it("fit → fit centered", () => {
    expect(backendToContent({ aspect_ratio_mode: "fit" })).toEqual({
      rotation: 0,
      fill: "fit",
      panX: 0.5,
      panY: 0.5,
      zoom: 1,
    });
  });
  it("undefined mode → stretch (backend default)", () => {
    expect(backendToContent({})).toEqual({
      rotation: 0,
      fill: "stretch",
      panX: 0.5,
      panY: 0.5,
      zoom: 1,
    });
  });
  it("clamps zoom and snaps rotation", () => {
    expect(
      backendToContent({
        rotation: 89,
        aspect_ratio_mode: "crop",
        crop: { x: 0.5, y: 0.5, scale: 0.3 },
      }),
    ).toEqual({ rotation: 90, fill: "cover", panX: 0.5, panY: 0.5, zoom: 1 });
  });
});

describe("fuzz: content ⇄ backend round-trips", () => {
  test.each(Array.from({ length: FUZZ }, (_, i) => i))(ITER, () => {
    const rotation = ([0, 90, 180, 270] as const)[randInt(0, 4)]!;
    const fill = (["cover", "fit", "stretch"] as const)[randInt(0, 3)]!;
    // 2-decimal pan/zoom so the round-trip is exact for cover
    const c: ContentTransform = {
      rotation,
      fill,
      panX: Math.round(rand(0, 1) * 100) / 100,
      panY: Math.round(rand(0, 1) * 100) / 100,
      zoom: Math.round(rand(1, 3) * 100) / 100,
    };
    const back = contentToBackend(c);
    expect(["stretch", "fit", "crop"]).toContain(back.aspect_ratio_mode);
    if (fill === "cover") {
      expect(back.crop).toBeDefined();
      expect(back.crop!.x).toBeGreaterThanOrEqual(0);
      expect(back.crop!.x).toBeLessThanOrEqual(1);
      expect(back.crop!.scale).toBeGreaterThanOrEqual(1);
    } else {
      expect(back.crop).toBeUndefined(); // fit/stretch must drop crop on the wire
    }
    const round = backendToContent(back);
    expect(round.rotation).toBe(rotation);
    expect(round.fill).toBe(fill);
    if (fill === "cover") {
      expect(round.panX).toBeCloseTo(c.panX, 6);
      expect(round.panY).toBeCloseTo(c.panY, 6);
      expect(round.zoom).toBeCloseTo(c.zoom, 6);
    } else {
      // fit/stretch normalize pan/zoom back to the centered identity
      expect(round.panX).toBe(0.5);
      expect(round.panY).toBe(0.5);
      expect(round.zoom).toBe(1);
    }
  });
});

// =====================================================================
// Group G — alignment candidates
// =====================================================================

describe("collectAlignmentCandidates", () => {
  const C = CANVAS;
  it("G1 includes canvas edges and centers", () => {
    const { vertical, horizontal } = collectAlignmentCandidates([], "r1", C);
    expect(vertical).toEqual([0, 960, 1920]);
    expect(horizontal).toEqual([0, 540, 1080]);
  });
  it("G2 adds other regions edges/centers, excludes the dragged one", () => {
    const regions: RegionRef[] = [
      { input: "r1", x: 0, y: 0, w: 400, h: 300 },
      { input: "r2", x: 960, y: 0, w: 960, h: 540 },
    ];
    const { vertical } = collectAlignmentCandidates(regions, "r1", C);
    expect(vertical).toContain(960); // r2.x (and canvas center)
    expect(vertical).toContain(1440); // r2 centerX
    expect(vertical).toContain(1920); // r2.right (and canvas right)
    // dragged r1's own edges (0,200,400) — 200/400 must not come from r1
    expect(vertical).not.toContain(200);
    expect(vertical).not.toContain(400);
  });
});
