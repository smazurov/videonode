import { describe, expect, it, test } from "vitest";

import { clamp, findAlignmentSnap, snapVal } from "./layout-math";

function rand(lo: number, hi: number) {
  // eslint-disable-next-line sonarjs/pseudo-random
  return lo + Math.random() * (hi - lo);
}
function randInt(lo: number, hi: number) {
  return Math.floor(rand(lo, hi));
}

const ITER = "iteration %i";
const FUZZ_ITERATIONS = 10_000;

describe("snapVal", () => {
  it("snaps to nearest grid multiple", () => {
    expect(snapVal(13, 10)).toBe(10);
    expect(snapVal(17, 10)).toBe(20);
    expect(snapVal(15, 10)).toBe(20);
    expect(snapVal(0, 10)).toBe(0);
    expect(snapVal(100, 10)).toBe(100);
  });
});

describe("clamp", () => {
  it("clamps below minimum", () => {
    expect(clamp(-5, 0, 100)).toBe(0);
  });
  it("clamps above maximum", () => {
    expect(clamp(150, 0, 100)).toBe(100);
  });
  it("passes through in range", () => {
    expect(clamp(50, 0, 100)).toBe(50);
  });
  it("handles boundary values", () => {
    expect(clamp(0, 0, 100)).toBe(0);
    expect(clamp(100, 0, 100)).toBe(100);
  });
});

describe("findAlignmentSnap", () => {
  it("returns null when no candidate is within threshold", () => {
    expect(findAlignmentSnap(100, 50, [200, 300])).toBeNull();
  });
  it("snaps left edge to candidate", () => {
    const r = findAlignmentSnap(98, 50, [100]);
    expect(r).not.toBeNull();
    expect(r!.snappedPos).toBe(100);
    expect(r!.guidePos).toBe(100);
  });
  it("snaps right edge to candidate", () => {
    const r = findAlignmentSnap(48, 50, [100]);
    expect(r).not.toBeNull();
    expect(r!.snappedPos).toBe(50);
    expect(r!.guidePos).toBe(100);
  });
  it("snaps center to candidate", () => {
    const r = findAlignmentSnap(73, 50, [100]);
    expect(r).not.toBeNull();
    expect(r!.snappedPos).toBe(75);
    expect(r!.guidePos).toBe(100);
  });
  it("picks closest match when multiple candidates qualify", () => {
    const r = findAlignmentSnap(99, 50, [100, 105]);
    expect(r).not.toBeNull();
    expect(r!.snappedPos).toBe(100);
  });
  it("picks closest edge when multiple edges match same candidate", () => {
    const r = findAlignmentSnap(97, 6, [100]);
    expect(r).not.toBeNull();
    expect(r!.snappedPos).toBe(97);
    expect(r!.guidePos).toBe(100);
  });
});

describe("fuzz: clamp invariants", () => {
  test.each(Array.from({ length: FUZZ_ITERATIONS }, (_, i) => i))(ITER, () => {
    const lo = rand(-10000, 10000);
    const hi = lo + rand(0, 10000);
    const v = rand(-20000, 20000);
    const c = clamp(v, lo, hi);
    expect(c).toBeGreaterThanOrEqual(lo);
    expect(c).toBeLessThanOrEqual(hi);
    if (v >= lo && v <= hi) expect(c).toBe(v);
  });
});

describe("fuzz: findAlignmentSnap invariants", () => {
  test.each(Array.from({ length: FUZZ_ITERATIONS }, (_, i) => i))(ITER, () => {
    const pos = rand(-2000, 6000);
    const size = rand(16, 4000);
    const numCands = randInt(0, 10);
    const candidates = Array.from({ length: numCands }, () =>
      rand(-1000, 8000),
    );
    const r = findAlignmentSnap(pos, size, candidates);
    if (r) {
      expect(candidates).toContainEqual(expect.closeTo(r.guidePos, 8));
      const leftDist = Math.abs(r.snappedPos - r.guidePos);
      const rightDist = Math.abs(r.snappedPos + size - r.guidePos);
      const centerDist = Math.abs(r.snappedPos + size / 2 - r.guidePos);
      expect(Math.min(leftDist, rightDist, centerDist)).toBeLessThanOrEqual(
        6.001,
      );
    }
  });
});
