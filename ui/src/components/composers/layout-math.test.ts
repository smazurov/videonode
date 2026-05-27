import { describe, expect, it, test } from 'vitest';

import {
  clamp,
  computeArPreview,
  computeCropPercent,
  findAlignmentSnap,
  konvaToVisual,
  snapVal,
  visualDims,
  visualToKonva,
} from './layout-math';

describe('computeArPreview', () => {
  it('returns null for stretch', () => {
    expect(computeArPreview(960, 540, 1920, 1080, 'stretch')).toBeNull();
  });

  it('returns null for undefined mode', () => {
    expect(computeArPreview(960, 540, 1920, 1080, undefined)).toBeNull();
  });

  it('returns null for invalid dimensions', () => {
    expect(computeArPreview(0, 540, 1920, 1080, 'fit')).toBeNull();
    expect(computeArPreview(960, 0, 1920, 1080, 'fit')).toBeNull();
    expect(computeArPreview(960, 540, 0, 1080, 'fit')).toBeNull();
    expect(computeArPreview(960, 540, 1920, 0, 'fit')).toBeNull();
    expect(computeArPreview(0, 540, 640, 480, 'crop')).toBeNull();
  });

  it('fit: pillarbox when source is taller (srcAr < slotAr)', () => {
    const r = computeArPreview(960, 540, 1080, 1920, 'fit')!;
    expect(r.w).toBeCloseTo(303.75);
    expect(r.h).toBe(540);
    expect(r.x).toBeCloseTo((960 - 303.75) / 2);
    expect(r.y).toBe(0);
  });

  it('fit: letterbox when source is wider (srcAr > slotAr)', () => {
    const r = computeArPreview(540, 540, 1920, 1080, 'fit')!;
    expect(r.w).toBe(540);
    expect(r.h).toBeCloseTo(303.75);
    expect(r.x).toBe(0);
    expect(r.y).toBeCloseTo((540 - 303.75) / 2);
  });

  it('crop: source wider than slot, centered by default', () => {
    // 1920x1080 (16:9) in 540x540 (1:1). srcAr > slotAr.
    // fillScale = max(540/1920, 540/1080) = max(0.281, 0.5) = 0.5
    // scaledW = 1920 * 0.5 = 960, scaledH = 1080 * 0.5 = 540
    // excessX = 960-540 = 420, centered: x = -210
    const r = computeArPreview(540, 540, 1920, 1080, 'crop')!;
    expect(r.w).toBeCloseTo(960);
    expect(r.h).toBeCloseTo(540);
    expect(r.x).toBeCloseTo(-210);
    expect(r.y).toBeCloseTo(0);
    // Source fully covers slot
    expect(r.x).toBeLessThanOrEqual(0);
    expect(r.x + r.w).toBeGreaterThanOrEqual(540);
    expect(r.y + r.h).toBeGreaterThanOrEqual(540);
  });

  it('crop: source taller than slot, centered by default', () => {
    // 640x480 (4:3) in 960x540 (16:9). srcAr < slotAr.
    // fillScale = max(960/640, 540/480) = max(1.5, 1.125) = 1.5
    // scaledW = 640 * 1.5 = 960, scaledH = 480 * 1.5 = 720
    // excessY = 720-540 = 180, centered: y = -90
    const r = computeArPreview(960, 540, 640, 480, 'crop')!;
    expect(r.w).toBeCloseTo(960);
    expect(r.h).toBeCloseTo(720);
    expect(r.x).toBeCloseTo(0);
    expect(r.y).toBeCloseTo(-90);
    // Source fully covers slot
    expect(r.x + r.w).toBeGreaterThanOrEqual(960);
    expect(r.y).toBeLessThanOrEqual(0);
    expect(r.y + r.h).toBeGreaterThanOrEqual(540);
  });

  it('crop: offset shifts the visible window', () => {
    const top = computeArPreview(960, 540, 640, 480, 'crop', 0.5, 0)!;
    expect(top.y).toBeCloseTo(0);
    const bottom = computeArPreview(960, 540, 640, 480, 'crop', 0.5, 1)!;
    expect(bottom.y).toBeCloseTo(-180);
    const center = computeArPreview(960, 540, 640, 480, 'crop', 0.5, 0.5)!;
    expect(center.y).toBeCloseTo(-90);
  });

  it('crop: horizontal offset for wider source', () => {
    const left = computeArPreview(540, 540, 1920, 1080, 'crop', 0)!;
    expect(left.x).toBeCloseTo(0);
    const right = computeArPreview(540, 540, 1920, 1080, 'crop', 1)!;
    expect(right.x).toBeCloseTo(-420);
  });
});

describe('computeCropPercent', () => {
  it('returns 100 for invalid dimensions', () => {
    expect(computeCropPercent(0, 540, 640, 480)).toBe(100);
    expect(computeCropPercent(960, 0, 640, 480)).toBe(100);
    expect(computeCropPercent(960, 540, 0, 480)).toBe(100);
    expect(computeCropPercent(960, 540, 640, 0)).toBe(100);
  });

  it('returns 75 for 4:3 source in 16:9 slot (srcAr < slotAr)', () => {
    expect(computeCropPercent(960, 540, 640, 480)).toBe(75);
  });

  it('returns 75 for 16:9 source in 4:3 slot (srcAr > slotAr)', () => {
    expect(computeCropPercent(640, 480, 1920, 1080)).toBe(75);
  });

  it('returns 100 for matching AR', () => {
    expect(computeCropPercent(960, 540, 1920, 1080)).toBe(100);
  });
});

describe('snapVal', () => {
  it('snaps to nearest grid multiple', () => {
    expect(snapVal(13, 10)).toBe(10);
    expect(snapVal(17, 10)).toBe(20);
    expect(snapVal(15, 10)).toBe(20);
    expect(snapVal(0, 10)).toBe(0);
    expect(snapVal(100, 10)).toBe(100);
  });
});

describe('clamp', () => {
  it('clamps below minimum', () => {
    expect(clamp(-5, 0, 100)).toBe(0);
  });

  it('clamps above maximum', () => {
    expect(clamp(150, 0, 100)).toBe(100);
  });

  it('passes through in range', () => {
    expect(clamp(50, 0, 100)).toBe(50);
  });

  it('handles boundary values', () => {
    expect(clamp(0, 0, 100)).toBe(0);
    expect(clamp(100, 0, 100)).toBe(100);
  });
});

describe('visualToKonva', () => {
  it('rotation 0: identity', () => {
    expect(visualToKonva(10, 20, 100, 50, 0)).toEqual({ x: 10, y: 20 });
  });

  it('rotation 90: x shifts right by h', () => {
    expect(visualToKonva(10, 20, 100, 50, 90)).toEqual({ x: 60, y: 20 });
  });

  it('rotation 180: shifts by w and h', () => {
    expect(visualToKonva(10, 20, 100, 50, 180)).toEqual({ x: 110, y: 70 });
  });

  it('rotation 270: y shifts down by w', () => {
    expect(visualToKonva(10, 20, 100, 50, 270)).toEqual({ x: 10, y: 120 });
  });

  it('handles negative rotation via modulo', () => {
    expect(visualToKonva(10, 20, 100, 50, -90)).toEqual(
      visualToKonva(10, 20, 100, 50, 270),
    );
  });
});

describe('konvaToVisual', () => {
  it('rotation 0: identity', () => {
    expect(konvaToVisual(10, 20, 100, 50, 0)).toEqual({ x: 10, y: 20 });
  });

  it('rotation 90: x shifts left by h', () => {
    expect(konvaToVisual(60, 20, 100, 50, 90)).toEqual({ x: 10, y: 20 });
  });

  it('rotation 180: shifts back by w and h', () => {
    expect(konvaToVisual(110, 70, 100, 50, 180)).toEqual({ x: 10, y: 20 });
  });

  it('rotation 270: y shifts up by w', () => {
    expect(konvaToVisual(10, 120, 100, 50, 270)).toEqual({ x: 10, y: 20 });
  });

  it('round-trips with visualToKonva', () => {
    for (const rot of [0, 90, 180, 270]) {
      const k = visualToKonva(30, 40, 200, 100, rot);
      const v = konvaToVisual(k.x, k.y, 200, 100, rot);
      expect(v).toEqual({ x: 30, y: 40 });
    }
  });
});

describe('visualDims', () => {
  it('rotation 0: unchanged', () => {
    expect(visualDims(960, 540, 0)).toEqual({ vw: 960, vh: 540 });
  });

  it('rotation 90: swapped', () => {
    expect(visualDims(960, 540, 90)).toEqual({ vw: 540, vh: 960 });
  });

  it('rotation 180: unchanged', () => {
    expect(visualDims(960, 540, 180)).toEqual({ vw: 960, vh: 540 });
  });

  it('rotation 270: swapped', () => {
    expect(visualDims(960, 540, 270)).toEqual({ vw: 540, vh: 960 });
  });
});

describe('findAlignmentSnap', () => {
  it('returns null when no candidate is within threshold', () => {
    expect(findAlignmentSnap(100, 50, [200, 300])).toBeNull();
  });

  it('snaps left edge to candidate', () => {
    const r = findAlignmentSnap(98, 50, [100]);
    expect(r).not.toBeNull();
    expect(r!.snappedPos).toBe(100);
    expect(r!.guidePos).toBe(100);
  });

  it('snaps right edge to candidate', () => {
    const r = findAlignmentSnap(48, 50, [100]);
    expect(r).not.toBeNull();
    expect(r!.snappedPos).toBe(50);
    expect(r!.guidePos).toBe(100);
  });

  it('snaps center to candidate', () => {
    const r = findAlignmentSnap(73, 50, [100]);
    expect(r).not.toBeNull();
    expect(r!.snappedPos).toBe(75);
    expect(r!.guidePos).toBe(100);
  });

  it('picks closest match when multiple candidates qualify', () => {
    const r = findAlignmentSnap(99, 50, [100, 105]);
    expect(r).not.toBeNull();
    expect(r!.snappedPos).toBe(100);
  });

  it('picks closest edge when multiple edges match same candidate', () => {
    // pos=97, size=6: left edge 97→100 (dist 3), right edge 103→100 (dist 3, snappedPos=94)
    // center 100→100 (dist 0, snappedPos=97). Center wins.
    const r = findAlignmentSnap(97, 6, [100]);
    expect(r).not.toBeNull();
    expect(r!.snappedPos).toBe(97);
    expect(r!.guidePos).toBe(100);
  });
});

// --- Fuzz tests ---

function rand(lo: number, hi: number) {
  // eslint-disable-next-line sonarjs/pseudo-random
  return lo + Math.random() * (hi - lo);
}

function randInt(lo: number, hi: number) {
  return Math.floor(rand(lo, hi));
}

describe('computeArPreview crop invariants', () => {
  it('source frame always covers output at scale 1', () => {
    const r = computeArPreview(960, 540, 640, 480, 'crop', 0.5, 0.5, 1)!;
    expect(r.w).toBeGreaterThanOrEqual(960);
    expect(r.h).toBeGreaterThanOrEqual(540);
  });

  it('source frame always covers output at high scale', () => {
    const r = computeArPreview(960, 540, 640, 480, 'crop', 0.5, 0.5, 2)!;
    expect(r.w).toBeGreaterThanOrEqual(960);
    expect(r.h).toBeGreaterThanOrEqual(540);
  });

  it('pan at extremes still covers output', () => {
    for (const cx of [0, 0.5, 1]) {
      for (const cy of [0, 0.5, 1]) {
        const r = computeArPreview(960, 540, 640, 480, 'crop', cx, cy, 1.5)!;
        // source left edge must be <= 0 (output starts at 0 in slot-local coords)
        expect(r.x).toBeLessThanOrEqual(0.001);
        // source top edge must be <= 0
        expect(r.y).toBeLessThanOrEqual(0.001);
        // source right edge must be >= slot width
        expect(r.x + r.w).toBeGreaterThanOrEqual(960 - 0.001);
        // source bottom edge must be >= slot height
        expect(r.y + r.h).toBeGreaterThanOrEqual(540 - 0.001);
      }
    }
  });
});

const ITER = 'iteration %i';

const FUZZ_ITERATIONS = 10_000;

describe('fuzz: visualToKonva / konvaToVisual round-trip', () => {
  test.each(Array.from({ length: FUZZ_ITERATIONS }, (_, i) => i))(
    ITER,
    () => {
      const vx = rand(-4000, 4000);
      const vy = rand(-4000, 4000);
      const w = rand(1, 8000);
      const h = rand(1, 4500);
      const rot = [0, 90, 180, 270][randInt(0, 4)]!;
      const k = visualToKonva(vx, vy, w, h, rot);
      const v = konvaToVisual(k.x, k.y, w, h, rot);
      expect(v.x).toBeCloseTo(vx, 8);
      expect(v.y).toBeCloseTo(vy, 8);
    },
  );
});

describe('fuzz: computeArPreview invariants', () => {
  test.each(Array.from({ length: FUZZ_ITERATIONS }, (_, i) => i))(
    ITER,
    () => {
      const slotW = rand(16, 7680);
      const slotH = rand(16, 4320);
      const srcW = rand(16, 7680);
      const srcH = rand(16, 4320);
      const mode = ['fit', 'crop', 'stretch', undefined][randInt(0, 4)];

      const cropX = rand(0, 1);
      const cropY = rand(0, 1);
      const cropScale = rand(0.5, 3);
      const r = computeArPreview(slotW, slotH, srcW, srcH, mode, cropX, cropY, cropScale);

      if (mode === 'stretch' || mode === undefined) {
        expect(r).toBeNull();
        return;
      }

      expect(r).not.toBeNull();
      const previewAr = r!.w / r!.h;
      const srcAr = srcW / srcH;
      expect(previewAr).toBeCloseTo(srcAr, 3);

      if (mode === 'fit') {
        expect(r!.x).toBeGreaterThanOrEqual(-0.001);
        expect(r!.y).toBeGreaterThanOrEqual(-0.001);
        expect(r!.x + r!.w).toBeLessThanOrEqual(slotW + 0.001);
        expect(r!.y + r!.h).toBeLessThanOrEqual(slotH + 0.001);
        const touchesW = Math.abs(r!.w - slotW) < 0.01;
        const touchesH = Math.abs(r!.h - slotH) < 0.01;
        expect(touchesW || touchesH).toBe(true);
      } else {
        // Crop: source must fully contain the output (slot)
        expect(r!.w).toBeGreaterThanOrEqual(slotW - 0.01);
        expect(r!.h).toBeGreaterThanOrEqual(slotH - 0.01);
        expect(r!.x).toBeLessThanOrEqual(0.001);
        expect(r!.y).toBeLessThanOrEqual(0.001);
        expect(r!.x + r!.w).toBeGreaterThanOrEqual(slotW - 0.001);
        expect(r!.y + r!.h).toBeGreaterThanOrEqual(slotH - 0.001);
      }
    },
  );
});

describe('fuzz: computeCropPercent invariants', () => {
  test.each(Array.from({ length: FUZZ_ITERATIONS }, (_, i) => i))(
    ITER,
    () => {
      const slotW = rand(16, 7680);
      const slotH = rand(16, 4320);
      const srcW = rand(16, 7680);
      const srcH = rand(16, 4320);

      const pct = computeCropPercent(slotW, slotH, srcW, srcH);

      expect(pct).toBeGreaterThanOrEqual(1);
      expect(pct).toBeLessThanOrEqual(100);
    },
  );
});

describe('fuzz: clamp invariants', () => {
  test.each(Array.from({ length: FUZZ_ITERATIONS }, (_, i) => i))(
    ITER,
    () => {
      const lo = rand(-10000, 10000);
      const hi = lo + rand(0, 10000);
      const v = rand(-20000, 20000);
      const c = clamp(v, lo, hi);
      expect(c).toBeGreaterThanOrEqual(lo);
      expect(c).toBeLessThanOrEqual(hi);
      if (v >= lo && v <= hi) expect(c).toBe(v);
    },
  );
});

describe('fuzz: visualDims invariants', () => {
  test.each(Array.from({ length: FUZZ_ITERATIONS }, (_, i) => i))(
    ITER,
    () => {
      const w = rand(1, 8000);
      const h = rand(1, 4500);
      const rot = [0, 90, 180, 270][randInt(0, 4)]!;
      const { vw, vh } = visualDims(w, h, rot);
      // Area is preserved
      expect(vw * vh).toBeCloseTo(w * h, 3);
      // Dimensions are one of the two original values
      expect([w, h]).toContainEqual(expect.closeTo(vw, 8));
      expect([w, h]).toContainEqual(expect.closeTo(vh, 8));
    },
  );
});

describe('fuzz: findAlignmentSnap invariants', () => {
  test.each(Array.from({ length: FUZZ_ITERATIONS }, (_, i) => i))(
    ITER,
    () => {
      const pos = rand(-2000, 6000);
      const size = rand(16, 4000);
      const numCands = randInt(0, 10);
      const candidates = Array.from({ length: numCands }, () => rand(-1000, 8000));

      const r = findAlignmentSnap(pos, size, candidates);

      if (r) {
        // guidePos must be one of the candidates
        expect(candidates).toContainEqual(expect.closeTo(r.guidePos, 8));
        // snappedPos must align one of the three edges to guidePos
        const leftDist = Math.abs(r.snappedPos - r.guidePos);
        const rightDist = Math.abs(r.snappedPos + size - r.guidePos);
        const centerDist = Math.abs(r.snappedPos + size / 2 - r.guidePos);
        const minDist = Math.min(leftDist, rightDist, centerDist);
        expect(minDist).toBeLessThanOrEqual(6.001);
      }
    },
  );
});
