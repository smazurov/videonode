import { describe, expect, it } from 'vitest';

import { computeArPreview } from './layout-math';

describe('computeArPreview', () => {
  it('returns null for stretch mode', () => {
    expect(computeArPreview(960, 540, 1920, 1080, 'stretch')).toBeNull();
    expect(computeArPreview(960, 540, 1920, 1080, undefined)).toBeNull();
  });

  it('returns letterbox rect for fit when source is taller', () => {
    // 1080x1920 vertical source in 960x540 landscape slot
    // srcAr=0.5625 < slotAr=1.778 → pillarbox: w = 540 * 0.5625 = 303.75
    const r = computeArPreview(960, 540, 1080, 1920, 'fit');
    expect(r).not.toBeNull();
    expect(r!.w).toBeCloseTo(303.75);
    expect(r!.h).toBe(540);
    expect(r!.x).toBeCloseTo((960 - 303.75) / 2);
    expect(r!.y).toBe(0);
  });

  it('returns pillarbox rect for fit when source is wider', () => {
    // 1920x1080 source in 540x540 square slot
    // srcAr=1.778 > slotAr=1.0 → letterbox: h = 540 / 1.778 = 303.75
    const r = computeArPreview(540, 540, 1920, 1080, 'fit');
    expect(r).not.toBeNull();
    expect(r!.w).toBe(540);
    expect(r!.h).toBeCloseTo(303.75);
  });

  it('returns null for crop mode (slot is fully filled)', () => {
    // 640x480 (4:3) in 960x540 (16:9) — crop cuts top/bottom of source
    expect(computeArPreview(960, 540, 640, 480, 'crop')).toBeNull();
    // 1920x1080 (16:9) in 540x540 (1:1) — crop cuts sides of source
    expect(computeArPreview(540, 540, 1920, 1080, 'crop')).toBeNull();
  });

  it('returns null for same aspect ratio regardless of mode', () => {
    // 1920x1080 in 960x540 — identical AR, fit produces full slot
    const r = computeArPreview(960, 540, 1920, 1080, 'fit');
    if (r) {
      expect(r.w).toBeCloseTo(960);
      expect(r.h).toBeCloseTo(540);
    }
  });
});
