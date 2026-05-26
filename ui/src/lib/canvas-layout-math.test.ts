import { describe, it, expect } from 'vitest';
import type { CanvasDims, LayoutSlot } from './composer-types';
import {
  applyCanvasBounds,
  applyHandleDelta,
  clampToCanvas,
  clampMove,
  clampResize,
  snap,
  snapSlot,
} from './canvas-layout-math';

const canvas: CanvasDims = { w: 500, h: 500 };

function slot(overrides: Partial<LayoutSlot> = {}): LayoutSlot {
  return { input: 'test', x: 100, y: 100, w: 200, h: 200, ...overrides };
}

describe('applyCanvasBounds', () => {
  describe('move handle', () => {
    it('clamps x to 0 without changing w', () => {
      const r = applyCanvasBounds(-20, 100, 200, 200, canvas, 'move');
      expect(r).toEqual({ x: 0, y: 100, w: 200, h: 200 });
    });

    it('clamps y to 0 without changing h', () => {
      const r = applyCanvasBounds(100, -30, 200, 200, canvas, 'move');
      expect(r).toEqual({ x: 100, y: 0, w: 200, h: 200 });
    });

    it('clamps x+w to canvas.w by shifting x', () => {
      const r = applyCanvasBounds(400, 100, 200, 200, canvas, 'move');
      expect(r).toEqual({ x: 300, y: 100, w: 200, h: 200 });
    });

    it('clamps y+h to canvas.h by shifting y', () => {
      const r = applyCanvasBounds(100, 400, 200, 200, canvas, 'move');
      expect(r).toEqual({ x: 100, y: 300, w: 200, h: 200 });
    });
  });

  describe('resize handles — right/bottom edge', () => {
    it('east: shrinks w when x+w exceeds canvas', () => {
      const r = applyCanvasBounds(100, 100, 500, 200, canvas, 'e');
      expect(r.x).toBe(100);
      expect(r.w).toBe(400);
    });

    it('south: shrinks h when y+h exceeds canvas', () => {
      const r = applyCanvasBounds(100, 100, 200, 500, canvas, 's');
      expect(r.y).toBe(100);
      expect(r.h).toBe(400);
    });
  });

  describe('resize handles — left/top edge', () => {
    it('west: shrinks w when x goes negative, preserves right edge', () => {
      const r = applyCanvasBounds(-20, 100, 420, 200, canvas, 'w');
      expect(r.x).toBe(0);
      expect(r.w).toBe(400);
      expect(r.x + r.w).toBe(400);
    });

    it('north: shrinks h when y goes negative, preserves bottom edge', () => {
      const r = applyCanvasBounds(100, -30, 200, 430, canvas, 'n');
      expect(r.y).toBe(0);
      expect(r.h).toBe(400);
      expect(r.y + r.h).toBe(400);
    });

    it('nw: shrinks both w and h when x,y go negative', () => {
      const r = applyCanvasBounds(-10, -20, 310, 420, canvas, 'nw');
      expect(r.x).toBe(0);
      expect(r.y).toBe(0);
      expect(r.w).toBe(300);
      expect(r.h).toBe(400);
    });
  });
});

describe('clampMove', () => {
  it('no-ops when slot is inside canvas', () => {
    const s = slot();
    expect(clampMove(s, canvas)).toEqual(s);
  });

  it('clamps position, preserves size', () => {
    const s = slot({ x: -10, y: -20 });
    const r = clampMove(s, canvas);
    expect(r.x).toBe(0);
    expect(r.y).toBe(0);
    expect(r.w).toBe(200);
    expect(r.h).toBe(200);
  });
});

describe('clampResize', () => {
  it('no-ops when slot is inside canvas', () => {
    const s = slot();
    expect(clampResize(s, canvas)).toEqual(s);
  });

  it('shrinks w when x is negative, preserves right edge', () => {
    const s = slot({ x: -15, w: 315 });
    const rightEdge = s.x + s.w;
    const r = clampResize(s, canvas);
    expect(r.x).toBe(0);
    expect(r.x + r.w).toBe(rightEdge);
  });

  it('shrinks h when y is negative, preserves bottom edge', () => {
    const s = slot({ y: -25, h: 325 });
    const bottomEdge = s.y + s.h;
    const r = clampResize(s, canvas);
    expect(r.y).toBe(0);
    expect(r.y + r.h).toBe(bottomEdge);
  });

  it('shrinks w when x+w exceeds canvas', () => {
    const s = slot({ x: 100, w: 450 });
    const r = clampResize(s, canvas);
    expect(r.x).toBe(100);
    expect(r.w).toBe(400);
  });
});

describe('clampToCanvas', () => {
  it('dispatches to clampMove for move handle', () => {
    const s = slot({ x: -10 });
    const r = clampToCanvas(s, canvas, 'move');
    expect(r.x).toBe(0);
    expect(r.w).toBe(200);
  });

  it('dispatches to clampResize for resize handles', () => {
    const s = slot({ x: -10, w: 310 });
    const r = clampToCanvas(s, canvas, 'w');
    expect(r.x).toBe(0);
    expect(r.w).toBe(300);
  });
});

describe('snap / snapSlot', () => {
  it('snap rounds to nearest grid', () => {
    expect(snap(17, 10)).toBe(20);
    expect(snap(14, 10)).toBe(10);
    expect(snap(15, 10)).toBe(20);
  });

  it('snapSlot snaps all four fields', () => {
    const s = slot({ x: 103, y: 97, w: 198, h: 205 });
    const r = snapSlot(s, 10);
    expect(r.x).toBe(100);
    expect(r.y).toBe(100);
    expect(r.w).toBe(200);
    expect(r.h).toBe(210);
  });
});

describe('applyHandleDelta — edge clamping preserves opposite edge', () => {
  const start = slot();

  it('west handle past left edge: right edge stays fixed', () => {
    const r = applyHandleDelta(start, 'w', -120, 0, canvas, false);
    expect(r.x).toBe(0);
    expect(r.x + r.w).toBe(start.x + start.w);
  });

  it('north handle past top edge: bottom edge stays fixed', () => {
    const r = applyHandleDelta(start, 'n', 0, -120, canvas, false);
    expect(r.y).toBe(0);
    expect(r.y + r.h).toBe(start.y + start.h);
  });

  it('east handle past right edge: left edge stays fixed', () => {
    const r = applyHandleDelta(start, 'e', 500, 0, canvas, false);
    expect(r.x).toBe(start.x);
    expect(r.x + r.w).toBe(canvas.w);
  });

  it('south handle past bottom edge: top edge stays fixed', () => {
    const r = applyHandleDelta(start, 's', 0, 500, canvas, false);
    expect(r.y).toBe(start.y);
    expect(r.y + r.h).toBe(canvas.h);
  });

  it('nw corner past top-left: bottom-right stays fixed', () => {
    const r = applyHandleDelta(start, 'nw', -200, -200, canvas, false);
    expect(r.x).toBe(0);
    expect(r.y).toBe(0);
    expect(r.x + r.w).toBe(start.x + start.w);
    expect(r.y + r.h).toBe(start.y + start.h);
  });

  it('se corner past bottom-right: top-left stays fixed', () => {
    const r = applyHandleDelta(start, 'se', 500, 500, canvas, false);
    expect(r.x).toBe(start.x);
    expect(r.y).toBe(start.y);
    expect(r.x + r.w).toBe(canvas.w);
    expect(r.y + r.h).toBe(canvas.h);
  });

  it('move past left edge: size preserved', () => {
    const r = applyHandleDelta(start, 'move', -200, 0, canvas, false);
    expect(r.x).toBe(0);
    expect(r.w).toBe(start.w);
    expect(r.h).toBe(start.h);
  });
});

describe('applyHandleDelta — rounding preserves fixed edge with fractional deltas', () => {
  const start = slot();

  it('west handle: right edge stable across fractional dx range', () => {
    const rightEdge = start.x + start.w;
    for (let dx = -10; dx >= -50; dx -= 0.3) {
      const r = applyHandleDelta(start, 'w', dx, 0, canvas, false);
      expect(r.x + r.w).toBe(rightEdge);
    }
  });

  it('north handle: bottom edge stable across fractional dy range', () => {
    const bottomEdge = start.y + start.h;
    for (let dy = -10; dy >= -50; dy -= 0.3) {
      const r = applyHandleDelta(start, 'n', 0, dy, canvas, false);
      expect(r.y + r.h).toBe(bottomEdge);
    }
  });

  it('east handle: left edge stable across fractional dx range', () => {
    for (let dx = 10; dx <= 50; dx += 0.3) {
      const r = applyHandleDelta(start, 'e', dx, 0, canvas, false);
      expect(r.x).toBe(start.x);
    }
  });

  it('south handle: top edge stable across fractional dy range', () => {
    for (let dy = 10; dy <= 50; dy += 0.3) {
      const r = applyHandleDelta(start, 's', 0, dy, canvas, false);
      expect(r.y).toBe(start.y);
    }
  });

  it('sw handle: right edge and top edge stable', () => {
    const rightEdge = start.x + start.w;
    for (let d = -10; d >= -50; d -= 0.3) {
      const r = applyHandleDelta(start, 'sw', d, -d, canvas, false);
      expect(r.x + r.w).toBe(rightEdge);
      expect(r.y).toBe(start.y);
    }
  });

  it('ne handle: left edge and bottom edge stable', () => {
    const bottomEdge = start.y + start.h;
    for (let d = 10; d <= 50; d += 0.3) {
      const r = applyHandleDelta(start, 'ne', d, -d, canvas, false);
      expect(r.x).toBe(start.x);
      expect(r.y + r.h).toBe(bottomEdge);
    }
  });
});

describe('snapSlot preserves fixed edges for resize handles', () => {
  it('west handle: right edge stays fixed across drag range', () => {
    const start = slot({ x: 100, w: 200 });
    const rightEdge = start.x + start.w;
    for (const dx of [-15, -20, -25, -30, -35]) {
      const after = applyHandleDelta(start, 'w', dx, 0, canvas, false);
      const snapped = snapSlot(after, 10, 'w');
      expect(snapped.x + snapped.w).toBe(rightEdge);
    }
  });

  it('north handle: bottom edge stays fixed across drag range', () => {
    const start = slot({ x: 100, y: 100, w: 200, h: 200 });
    const bottomEdge = start.y + start.h;
    for (const dy of [-15, -20, -25, -30, -35]) {
      const after = applyHandleDelta(start, 'n', 0, dy, canvas, false);
      const snapped = snapSlot(after, 10, 'n');
      expect(snapped.y + snapped.h).toBe(bottomEdge);
    }
  });

  it('east handle: left edge stays fixed', () => {
    const start = slot({ x: 100, w: 200 });
    for (const dx of [13, 17, 23, 27]) {
      const after = applyHandleDelta(start, 'e', dx, 0, canvas, false);
      const snapped = snapSlot(after, 10, 'e');
      expect(snapped.x).toBe(start.x);
    }
  });

  it('south handle: top edge stays fixed', () => {
    const start = slot({ x: 100, y: 100, w: 200, h: 200 });
    for (const dy of [13, 17, 23, 27]) {
      const after = applyHandleDelta(start, 's', 0, dy, canvas, false);
      const snapped = snapSlot(after, 10, 's');
      expect(snapped.y).toBe(start.y);
    }
  });

  it('nw handle: bottom-right corner stays fixed', () => {
    const start = slot({ x: 100, y: 100, w: 200, h: 200 });
    const rightEdge = start.x + start.w;
    const bottomEdge = start.y + start.h;
    const after = applyHandleDelta(start, 'nw', -15, -15, canvas, false);
    const snapped = snapSlot(after, 10, 'nw');
    expect(snapped.x + snapped.w).toBe(rightEdge);
    expect(snapped.y + snapped.h).toBe(bottomEdge);
  });

  it('se handle: top-left corner stays fixed', () => {
    const start = slot({ x: 100, y: 100, w: 200, h: 200 });
    const after = applyHandleDelta(start, 'se', 13, 17, canvas, false);
    const snapped = snapSlot(after, 10, 'se');
    expect(snapped.x).toBe(start.x);
    expect(snapped.y).toBe(start.y);
  });

  it('move handle: snaps all four values independently (no fixed edge)', () => {
    const s = slot({ x: 103, y: 97, w: 198, h: 205 });
    const snapped = snapSlot(s, 10, 'move');
    expect(snapped.x).toBe(100);
    expect(snapped.y).toBe(100);
    expect(snapped.w).toBe(200);
    expect(snapped.h).toBe(210);
  });
});

describe('snapSlot + clampToCanvas interaction', () => {
  it('snap pushes slot past right edge, clamp constrains w not x', () => {
    const s = slot({ x: 100, w: 395 });
    const snapped = snapSlot(s, 10, 'e');
    expect(snapped.w).toBe(400);
    const clamped = clampToCanvas(snapped, canvas, 'e');
    expect(clamped.x).toBe(100);
    expect(clamped.w).toBe(400);
  });

  it('snap pushes x negative, clamp constrains w not position', () => {
    const s = slot({ x: 3, w: 297 });
    const snapped = snapSlot(s, 10, 'w');
    expect(snapped.x).toBe(0);
    const clamped = clampToCanvas(snapped, canvas, 'w');
    expect(clamped.x).toBe(0);
    expect(clamped.w).toBe(300);
  });
});
