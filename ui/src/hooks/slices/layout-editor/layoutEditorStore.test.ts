// Store-level coverage for the layout editor.
//
// These cases were converted from the deleted Playwright harness spec
// (tests/layout-editor.spec.ts). They cover the NON-geometry integration the
// store owns — default layout, canvas-preset reset, selection, undo/redo,
// nudge-as-commit, and content-mode mapping. The geometry math (clamp/resize/
// rotation/fill/crop) lives in region-content.test.ts and is not retested here.

import { beforeEach, describe, expect, it } from "vitest";

import type { CanvasDims, LayoutSlot } from "../../../lib/composer-types";
import type { ContentTransform } from "../../../components/composers/region-content";
import { useLayoutEditorStore } from "../../useLayoutEditorStore";

// Mirror of the harness's defaultLayout(canvas): a four-up quad split.
const SRC = "source:cam-";
function defaultLayout(canvas: CanvasDims): LayoutSlot[] {
  const hw = Math.floor(canvas.w / 2);
  const hh = Math.floor(canvas.h / 2);
  return [
    { input: `${SRC}1`, x: 0, y: 0, w: hw, h: hh },
    { input: `${SRC}2`, x: hw, y: 0, w: hw, h: hh },
    { input: `${SRC}3`, x: 0, y: hh, w: hw, h: hh },
    { input: `${SRC}4`, x: hw, y: hh, w: hw, h: hh },
  ];
}

const HD: CanvasDims = { w: 1920, h: 1080 };

// Direct store handle — these are plain Zustand actions, no React needed.
const store = useLayoutEditorStore;
const get = () => store.getState();

beforeEach(() => {
  // Reset to a known default-quad layout on the 1080p canvas before each case.
  store.setState({ canvas: { ...HD } });
  get().resetHistory(defaultLayout(HD));
  get().select(`${SRC}1`);
});

// =====================================================================
// Default layout (from "renders four slots in default quad layout")
// =====================================================================

describe("default layout", () => {
  it("resetHistory seeds the four-up quad split for 1080p", () => {
    const { layout } = get();
    expect(layout).toHaveLength(4);
    expect(layout[0]).toMatchObject({
      input: `${SRC}1`,
      x: 0,
      y: 0,
      w: 960,
      h: 540,
    });
    expect(layout[1]).toMatchObject({
      input: `${SRC}2`,
      x: 960,
      y: 0,
      w: 960,
      h: 540,
    });
    expect(layout[2]).toMatchObject({
      input: `${SRC}3`,
      x: 0,
      y: 540,
      w: 960,
      h: 540,
    });
    expect(layout[3]).toMatchObject({
      input: `${SRC}4`,
      x: 960,
      y: 540,
      w: 960,
      h: 540,
    });
  });

  it("resetHistory clears past/future", () => {
    get().commitSlot(`${SRC}1`, { x: 100 });
    expect(get().past.length).toBeGreaterThan(0);
    get().resetHistory(defaultLayout(HD));
    expect(get().past).toHaveLength(0);
    expect(get().future).toHaveLength(0);
  });
});

// =====================================================================
// Selection (from "clicking a slot selects it in the inspector")
// =====================================================================

describe("selection", () => {
  it("select stores the active input ref", () => {
    get().select(`${SRC}2`);
    expect(get().selectedInput).toBe(`${SRC}2`);
  });

  it("select(null) clears the selection", () => {
    get().select(null);
    expect(get().selectedInput).toBeNull();
  });
});

// =====================================================================
// Nudge (from "keyboard arrow nudges selected slot")
// The arrow-key handler lives in the canvas component; the state change it
// produces is a commitSlot of x/y by the grid step. Test that store effect.
// =====================================================================

describe("nudge via commitSlot", () => {
  it("ArrowRight-equivalent shifts x by the grid step, y unchanged", () => {
    const before = get().layout[0]!;
    get().commitSlot(`${SRC}1`, { x: before.x + 10 });
    const after = get().layout.find((s) => s.input === `${SRC}1`)!;
    expect(after.x).toBe(before.x + 10);
    expect(after.y).toBe(before.y);
  });

  it("ArrowDown-equivalent shifts y by the grid step, x unchanged", () => {
    const before = get().layout[0]!;
    get().commitSlot(`${SRC}1`, { y: before.y + 10 });
    const after = get().layout.find((s) => s.input === `${SRC}1`)!;
    expect(after.y).toBe(before.y + 10);
    expect(after.x).toBe(before.x);
  });

  it("commitSlot only mutates the targeted slot", () => {
    get().commitSlot(`${SRC}1`, { x: 500 });
    const others = get().layout.filter((s) => s.input !== `${SRC}1`);
    expect(others).toEqual(defaultLayout(HD).slice(1));
  });
});

// =====================================================================
// Reset (from "reset button restores default layout")
// =====================================================================

describe("reset to default", () => {
  it("restores the default quad after edits", () => {
    get().commitSlot(`${SRC}1`, { x: 10 });
    expect(get().layout[0]!.x).toBe(10);

    get().resetHistory(defaultLayout(get().canvas));
    get().select(`${SRC}1`);
    expect(get().layout[0]).toMatchObject({ x: 0, y: 0, w: 960, h: 540 });
  });
});

// =====================================================================
// Canvas preset (from "canvas preset changes dimensions and resets layout")
// =====================================================================

describe("canvas preset switch", () => {
  it("720p preset resizes the canvas and re-lays the default quad", () => {
    const p720: CanvasDims = { w: 1280, h: 720 };
    get().setCanvas(p720);
    get().resetHistory(defaultLayout(p720));

    expect(get().canvas).toEqual(p720);
    const { layout } = get();
    expect(layout).toHaveLength(4);
    // 720p → half-sizes 640x360
    expect(layout[0]).toMatchObject({ w: 640, h: 360 });
    expect(layout[3]).toMatchObject({ x: 640, y: 360, w: 640, h: 360 });
  });

  it("setCanvas alone does not touch the layout", () => {
    const edited = get().layout;
    get().setCanvas({ w: 1280, h: 720 });
    expect(get().layout).toBe(edited);
  });
});

// =====================================================================
// Undo / redo (from "undo reverts...", "redo restores...", "undo button works")
// =====================================================================

describe("undo / redo", () => {
  it("undo reverts the last commit", () => {
    get().commitSlot(`${SRC}1`, { x: 10 });
    expect(get().layout[0]!.x).toBe(10);
    get().undo();
    expect(get().layout[0]!.x).toBe(0);
  });

  it("redo restores an undone commit", () => {
    get().commitSlot(`${SRC}1`, { x: 10 });
    get().undo();
    expect(get().layout[0]!.x).toBe(0);
    get().redo();
    expect(get().layout[0]!.x).toBe(10);
  });

  it("undo on the y-axis (ArrowDown then undo)", () => {
    get().commitSlot(`${SRC}1`, { y: 10 });
    expect(get().layout[0]!.y).toBe(10);
    get().undo();
    expect(get().layout[0]!.y).toBe(0);
  });

  it("undo with empty history is a no-op", () => {
    const before = get().layout;
    get().undo();
    expect(get().layout).toBe(before);
  });

  it("redo with empty future is a no-op", () => {
    const before = get().layout;
    get().redo();
    expect(get().layout).toBe(before);
  });

  it("a fresh commit clears the redo stack", () => {
    get().commitSlot(`${SRC}1`, { x: 10 });
    get().undo();
    expect(get().future.length).toBe(1);
    get().commitSlot(`${SRC}1`, { x: 20 });
    expect(get().future).toHaveLength(0);
    get().undo();
    expect(get().layout[0]!.x).toBe(0); // the original, not 10
  });

  it("multi-step undo/redo walks the stack", () => {
    get().commitSlot(`${SRC}1`, { x: 10 });
    get().commitSlot(`${SRC}1`, { x: 20 });
    get().commitSlot(`${SRC}1`, { x: 30 });
    expect(get().layout[0]!.x).toBe(30);
    get().undo();
    get().undo();
    expect(get().layout[0]!.x).toBe(10);
    get().redo();
    expect(get().layout[0]!.x).toBe(20);
  });
});

// =====================================================================
// Content mode (from "changing aspect ratio mode updates layout state")
// commitContent maps the editor ContentTransform onto the wire slot fields.
// =====================================================================

describe("commitContent → wire fields", () => {
  const content = (over: Partial<ContentTransform>): ContentTransform => ({
    rotation: 0,
    fill: "cover",
    panX: 0.5,
    panY: 0.5,
    zoom: 1,
    ...over,
  });

  it("fit mode sets aspect_ratio_mode=fit and omits crop", () => {
    get().commitContent(`${SRC}1`, content({ fill: "fit" }));
    const slot = get().layout.find((s) => s.input === `${SRC}1`)!;
    expect(slot.aspect_ratio_mode).toBe("fit");
    expect(slot.crop).toBeUndefined();
  });

  it("cover mode sets aspect_ratio_mode=crop with a crop window", () => {
    get().commitContent(
      `${SRC}1`,
      content({ fill: "cover", panX: 0.2, panY: 0.8, zoom: 1.5 }),
    );
    const slot = get().layout.find((s) => s.input === `${SRC}1`)!;
    expect(slot.aspect_ratio_mode).toBe("crop");
    expect(slot.crop).toEqual({ x: 0.2, y: 0.8, scale: 1.5 });
  });

  it("switching cover → fit drops the stale crop", () => {
    get().commitContent(`${SRC}1`, content({ fill: "cover" }));
    expect(get().layout.find((s) => s.input === `${SRC}1`)!.crop).toBeDefined();
    get().commitContent(`${SRC}1`, content({ fill: "fit" }));
    expect(
      get().layout.find((s) => s.input === `${SRC}1`)!.crop,
    ).toBeUndefined();
  });

  it("rotation is carried onto the slot", () => {
    get().commitContent(`${SRC}1`, content({ rotation: 90 }));
    expect(get().layout.find((s) => s.input === `${SRC}1`)!.rotation).toBe(90);
  });

  it("commitContent is undoable", () => {
    get().commitContent(`${SRC}1`, content({ fill: "fit" }));
    expect(
      get().layout.find((s) => s.input === `${SRC}1`)!.aspect_ratio_mode,
    ).toBe("fit");
    get().undo();
    expect(
      get().layout.find((s) => s.input === `${SRC}1`)!.aspect_ratio_mode,
    ).toBeUndefined();
  });

  it("commitContent only mutates the targeted slot", () => {
    const others = get()
      .layout.filter((s) => s.input !== `${SRC}1`)
      .map((s) => ({ ...s }));
    get().commitContent(`${SRC}1`, content({ fill: "fit", rotation: 180 }));
    for (const before of others) {
      expect(get().layout.find((s) => s.input === before.input)).toEqual(
        before,
      );
    }
  });
});
