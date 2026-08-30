import { describe, expect, it } from "vitest";

import {
  buildSourceDims,
  clipPathData,
  encodeCanvasSize,
  encodeClip,
  layoutFootprints,
  parseCanvasSize,
  parseClip,
  slotFootprint,
} from "./canvas-mask";
import type { Source } from "../../hooks/slices/types";
import type { LayoutSlot } from "../../lib/composer-types";

const CANVAS = { w: 1920, h: 1080 };
const HD = { w: 1920, h: 1080 };
const CAM = "source:cam";

function slot(over: Partial<LayoutSlot> = {}): LayoutSlot {
  return { input: CAM, x: 0, y: 0, w: 960, h: 540, ...over };
}

describe("slotFootprint", () => {
  it("fills the region in stretch mode", () => {
    expect(slotFootprint(slot({ aspect_ratio_mode: "stretch" }), { w: 1280, h: 720 })).toEqual({
      x: 0,
      y: 0,
      w: 960,
      h: 540,
    });
  });

  it("fills the region when the mode is absent (backend default)", () => {
    expect(slotFootprint(slot(), { w: 1280, h: 720 })).toEqual({ x: 0, y: 0, w: 960, h: 540 });
  });

  it("fills the region in crop mode, ignoring pan and zoom", () => {
    const s = slot({ aspect_ratio_mode: "crop", crop: { x: 0.1, y: 0.9, scale: 2 } });
    expect(slotFootprint(s, { w: 1280, h: 720 })).toEqual({ x: 0, y: 0, w: 960, h: 540 });
  });

  it("pillarboxes in fit mode when the source is narrower than the region", () => {
    // 1:1 source in a 2:1 region → full height, centered width.
    const s = slot({ aspect_ratio_mode: "fit" });
    expect(slotFootprint(s, { w: 1000, h: 1000 })).toEqual({ x: 210, y: 0, w: 540, h: 540 });
  });

  it("letterboxes in fit mode when the source is wider than the region", () => {
    // 16:9 source in a 1:1 region → full width, centered height.
    const s = slot({ aspect_ratio_mode: "fit", w: 540, h: 540 });
    expect(slotFootprint(s, HD)).toEqual({ x: 0, y: 118.125, w: 540, h: 303.75 });
  });

  it("is offset by the slot origin", () => {
    const s = slot({ x: 300, y: 200, aspect_ratio_mode: "fit" });
    expect(slotFootprint(s, { w: 1000, h: 1000 })).toEqual({ x: 510, y: 200, w: 540, h: 540 });
  });

  it("inverts the source aspect ratio at 90 and 270 degrees", () => {
    // 16:9 source rotated 90° displays as 9:16, which pillarboxes in a 16:9 region.
    const s = slot({ aspect_ratio_mode: "fit", rotation: 90 });
    const at90 = slotFootprint(s, HD);
    expect(at90).toEqual({ x: 328.125, y: 0, w: 303.75, h: 540 });
    expect(slotFootprint({ ...s, rotation: 270 }, HD)).toEqual(at90);
  });

  it("is unchanged by 180 degrees", () => {
    const s = slot({ aspect_ratio_mode: "fit" });
    expect(slotFootprint({ ...s, rotation: 180 }, { w: 1000, h: 1000 })).toEqual(
      slotFootprint(s, { w: 1000, h: 1000 }),
    );
  });

  it("falls back to the full region when the source size is unknown", () => {
    // Mirrors build_source_slot.cpp's own `frame.width > 0` guard.
    const s = slot({ aspect_ratio_mode: "fit" });
    expect(slotFootprint(s, { w: 0, h: 0 })).toEqual({ x: 0, y: 0, w: 960, h: 540 });
  });
});

describe("layoutFootprints", () => {
  it("returns one rect per slot", () => {
    const layout = [
      slot({ input: "source:a" }),
      slot({ input: "source:b", x: 960, y: 540 }),
    ];
    expect(layoutFootprints(layout, CANVAS)).toEqual([
      { x: 0, y: 0, w: 960, h: 540 },
      { x: 960, y: 540, w: 960, h: 540 },
    ]);
  });

  it("clips slots that hang off the canvas", () => {
    const layout = [slot({ x: -200, y: -100, w: 960, h: 540 })];
    expect(layoutFootprints(layout, CANVAS)).toEqual([{ x: 0, y: 0, w: 760, h: 440 }]);
  });

  it("drops slots entirely off the canvas", () => {
    expect(layoutFootprints([slot({ x: 2000 })], CANVAS)).toEqual([]);
  });

  it("uses the source dims map when the ref is present", () => {
    const dims = new Map([[CAM, { w: 1000, h: 1000 }]]);
    const layout = [slot({ aspect_ratio_mode: "fit" })];
    expect(layoutFootprints(layout, CANVAS, dims)).toEqual([{ x: 210, y: 0, w: 540, h: 540 }]);
  });

  it("falls back to the region size for refs missing from the map", () => {
    const layout = [slot({ aspect_ratio_mode: "fit" })];
    expect(layoutFootprints(layout, CANVAS, new Map())).toEqual([
      { x: 0, y: 0, w: 960, h: 540 },
    ]);
  });
});

describe("clipPathData", () => {
  it("emits one normalized subpath per rect", () => {
    const path = clipPathData([{ x: 960, y: 0, w: 960, h: 540 }], CANVAS);
    expect(path).toBe("M0.5 0H1V0.5H0.5Z");
  });

  it("joins disjoint rects into a single path", () => {
    const path = clipPathData(
      [
        { x: 0, y: 0, w: 960, h: 1080 },
        { x: 960, y: 0, w: 960, h: 540 },
      ],
      CANVAS,
    );
    expect(path).toBe("M0 0H0.5V1H0Z M0.5 0H1V0.5H0.5Z");
  });

  it("returns empty for a degenerate canvas", () => {
    expect(clipPathData([{ x: 0, y: 0, w: 10, h: 10 }], { w: 0, h: 0 })).toBe("");
  });
});

describe("clip encoding", () => {
  it("round-trips rects", () => {
    const rects = [
      { x: 0, y: 0, w: 960, h: 540 },
      { x: 960, y: 540, w: 960, h: 540 },
    ];
    expect(parseClip(encodeClip(rects))).toEqual(rects);
  });

  it("rounds fractional rects on encode", () => {
    expect(encodeClip([{ x: 0, y: 118.125, w: 540, h: 303.75 }])).toBe("0,118,540,304");
  });

  it("returns empty for missing or malformed input", () => {
    expect(parseClip(null)).toEqual([]);
    expect(parseClip("")).toEqual([]);
    expect(parseClip("1,2,3")).toEqual([]);
    expect(parseClip("a,b,c,d")).toEqual([]);
    expect(parseClip("0,0,0,540")).toEqual([]);
  });

  it("skips malformed entries but keeps valid ones", () => {
    expect(parseClip("0,0,960,540;bogus;960,0,960,540")).toEqual([
      { x: 0, y: 0, w: 960, h: 540 },
      { x: 960, y: 0, w: 960, h: 540 },
    ]);
  });
});

describe("canvas size encoding", () => {
  it("round-trips", () => {
    expect(parseCanvasSize(encodeCanvasSize(CANVAS))).toEqual(CANVAS);
  });

  it("rejects malformed sizes", () => {
    expect(parseCanvasSize(null)).toBeNull();
    expect(parseCanvasSize("1920")).toBeNull();
    expect(parseCanvasSize("1920x")).toBeNull();
    expect(parseCanvasSize("0x1080")).toBeNull();
  });
});

describe("buildSourceDims", () => {
  const asSource = (s: unknown) => s as Source;

  it("prefers the live negotiated format over the configured one", () => {
    const dims = buildSourceDims([{ ref: CAM }], {
      cam: asSource({ latest_status: { format: { w: 1280, h: 720 } }, format: { width: 1920, height: 1080 } }),
    });
    expect(dims.get(CAM)).toEqual({ w: 1280, h: 720 });
  });

  it("falls back to the configured format", () => {
    const dims = buildSourceDims([{ ref: CAM }], {
      cam: asSource({ format: { width: 1920, height: 1080 } }),
    });
    expect(dims.get(CAM)).toEqual(HD);
  });

  it("omits refs with no known size", () => {
    expect(buildSourceDims([{ ref: CAM }], {}).size).toBe(0);
    expect(buildSourceDims([{ ref: CAM }], { cam: asSource({}) }).size).toBe(0);
  });
});
