import { test, expect, type Page } from '@playwright/test';

// Canvas is 1920x1080 by default. The Konva stage scales to fit its container.
// These helpers convert canvas-pixel coords to screen-pixel coords at runtime.

interface CanvasGeometry {
  stageX: number;
  stageY: number;
  scale: number;
}

async function getCanvasGeometry(page: Page): Promise<CanvasGeometry> {
  const rect = await page
    .locator('[data-testid="canvas-container"] canvas')
    .boundingBox();
  if (!rect) throw new Error('Canvas not found');
  return {
    stageX: rect.x,
    stageY: rect.y,
    scale: rect.width / 1920,
  };
}

function canvasToScreen(
  geo: CanvasGeometry,
  cx: number,
  cy: number,
): { x: number; y: number } {
  return {
    x: geo.stageX + cx * geo.scale,
    y: geo.stageY + cy * geo.scale,
  };
}

async function getLayoutState(page: Page) {
  const text = await page
    .locator('[data-testid="layout-debug"]')
    .textContent();
  return JSON.parse(text!) as Array<{
    input: string;
    x: number;
    y: number;
    w: number;
    h: number;
    rotation?: number;
  }>;
}

async function dragOnCanvas(
  page: Page,
  geo: CanvasGeometry,
  from: { cx: number; cy: number },
  to: { cx: number; cy: number },
) {
  const start = canvasToScreen(geo, from.cx, from.cy);
  const end = canvasToScreen(geo, to.cx, to.cy);
  await page.mouse.move(start.x, start.y);
  await page.mouse.down();
  // Move in small steps so Konva fires dragMove events
  const steps = 10;
  for (let i = 1; i <= steps; i++) {
    await page.mouse.move(
      start.x + ((end.x - start.x) * i) / steps,
      start.y + ((end.y - start.y) * i) / steps,
    );
  }
  await page.mouse.up();
}

test.beforeEach(async ({ page }) => {
  await page.goto('/test/layout-editor');
  await page.waitForSelector('[data-testid="canvas-container"] canvas');
});

test.describe('layout editor canvas', () => {
  test('renders four slots in default quad layout', async ({ page }) => {
    const layout = await getLayoutState(page);
    expect(layout).toHaveLength(4);
    expect(layout[0]).toMatchObject({ input: 'source:cam-1', x: 0, y: 0, w: 960, h: 540 });
    expect(layout[1]).toMatchObject({ input: 'source:cam-2', x: 960, y: 0, w: 960, h: 540 });
    expect(layout[2]).toMatchObject({ input: 'source:cam-3', x: 0, y: 540, w: 960, h: 540 });
    expect(layout[3]).toMatchObject({ input: 'source:cam-4', x: 960, y: 540, w: 960, h: 540 });
  });

  test('clicking a slot selects it in the inspector', async ({ page }) => {
    const geo = await getCanvasGeometry(page);
    // Click center of slot 2 (top-right quadrant)
    const s2center = canvasToScreen(geo, 960 + 480, 270);
    await page.mouse.click(s2center.x, s2center.y);
    // Inspector should show source:cam-2
    await expect(page.locator('select[name], .slot-inspector select, select').first())
      .toHaveValue('source:cam-2');
  });

  test('dragging a slot updates inspector X/Y values', async ({ page }) => {
    const geo = await getCanvasGeometry(page);
    const s1center = canvasToScreen(geo, 480, 270);
    await page.mouse.click(s1center.x, s1center.y);

    // Read inspector X before drag
    const xBefore = await page.getByRole('spinbutton', { name: 'X', exact: true }).inputValue();

    await dragOnCanvas(page, geo, { cx: 480, cy: 270 }, { cx: 580, cy: 270 });

    // Inspector X should now reflect the dragged position
    const xAfter = await page.getByRole('spinbutton', { name: 'X', exact: true }).inputValue();
    expect(Number(xAfter)).toBeGreaterThan(Number(xBefore));
  });

  test('dragging a slot updates its position', async ({ page }) => {
    const geo = await getCanvasGeometry(page);
    // First click slot 1 to select it
    const s1center = canvasToScreen(geo, 480, 270);
    await page.mouse.click(s1center.x, s1center.y);

    // Drag slot 1 from its center to (100, 100) in canvas coords
    await dragOnCanvas(page, geo, { cx: 480, cy: 270 }, { cx: 580, cy: 370 });

    const layout = await getLayoutState(page);
    const slot1 = layout.find((s) => s.input === 'source:cam-1')!;
    // Should have moved ~100px in each direction (snap to grid may round)
    expect(slot1.x).toBeGreaterThan(50);
    expect(slot1.x).toBeLessThan(150);
    expect(slot1.y).toBeGreaterThan(50);
    expect(slot1.y).toBeLessThan(150);
  });

  test('slot cannot be dragged outside canvas bounds', async ({ page }) => {
    const geo = await getCanvasGeometry(page);
    // Select slot 1
    const s1center = canvasToScreen(geo, 480, 270);
    await page.mouse.click(s1center.x, s1center.y);

    // Try to drag slot 1 way past the right edge
    await dragOnCanvas(page, geo, { cx: 480, cy: 270 }, { cx: 1800, cy: 270 });

    const layout = await getLayoutState(page);
    const slot1 = layout.find((s) => s.input === 'source:cam-1')!;
    // x + w should not exceed canvas width (1920)
    expect(slot1.x + slot1.w).toBeLessThanOrEqual(1920);
  });

  test('dragging slot near canvas center lands on grid, not alignment snap', async ({
    page,
  }) => {
    const geo = await getCanvasGeometry(page);

    // Remove slots 2,3,4 so only slot 1 remains.
    for (const ref of ['source:cam-2', 'source:cam-3', 'source:cam-4']) {
      const layout = await getLayoutState(page);
      const slot = layout.find((s) => s.input === ref)!;
      const center = canvasToScreen(geo, slot.x + slot.w / 2, slot.y + slot.h / 2);
      await page.mouse.click(center.x, center.y);
      await page.click('[data-testid="remove-slot"]');
    }

    // Drag slot 1 toward center. Without alignment snap, final position
    // is determined by grid snap (default 10px grid).
    await dragOnCanvas(page, geo, { cx: 480, cy: 270 }, { cx: 960, cy: 540 });

    const layout = await getLayoutState(page);
    const slot1 = layout.find((s) => s.input === 'source:cam-1')!;
    // Position should be grid-snapped (divisible by 10), NOT alignment-snapped to center
    expect(slot1.x % 10).toBe(0);
    expect(slot1.y % 10).toBe(0);
  });

  test('dragging slot does not alignment-snap to other slot edges', async ({
    page,
  }) => {
    const geo = await getCanvasGeometry(page);

    // Slot 1 is at (0,0,960,540). Slot 2 is at (960,0,960,540).
    // Drag slot 2 left by 4px — within old snap threshold but should NOT snap now.
    const s2center = canvasToScreen(geo, 1440, 270);
    await page.mouse.click(s2center.x, s2center.y);

    await dragOnCanvas(page, geo, { cx: 1440, cy: 270 }, { cx: 1436, cy: 270 });

    const layout = await getLayoutState(page);
    const slot2 = layout.find((s) => s.input === 'source:cam-2')!;
    // Should grid-snap to 960 (nearest 10), NOT alignment-snap
    // The drag target is 960-4=956, grid snaps to 960
    expect(slot2.x).toBe(960);
  });

  test('keyboard arrow nudges selected slot', async ({ page }) => {
    const geo = await getCanvasGeometry(page);
    // Select slot 1
    const s1center = canvasToScreen(geo, 480, 270);
    await page.mouse.click(s1center.x, s1center.y);

    const before = await getLayoutState(page);
    const slot1Before = before.find((s) => s.input === 'source:cam-1')!;

    // Press ArrowRight — should nudge by grid size (10)
    await page.keyboard.press('ArrowRight');
    const after = await getLayoutState(page);
    const slot1After = after.find((s) => s.input === 'source:cam-1')!;
    expect(slot1After.x).toBe(slot1Before.x + 10);
    expect(slot1After.y).toBe(slot1Before.y);
  });

  test('reset button restores default layout', async ({ page }) => {
    const geo = await getCanvasGeometry(page);
    // Move slot 1
    const s1center = canvasToScreen(geo, 480, 270);
    await page.mouse.click(s1center.x, s1center.y);
    await page.keyboard.press('ArrowRight');

    // Verify it moved
    let layout = await getLayoutState(page);
    expect(layout[0]!.x).toBe(10);

    // Reset
    await page.click('[data-testid="reset-layout"]');
    layout = await getLayoutState(page);
    expect(layout[0]).toMatchObject({ x: 0, y: 0, w: 960, h: 540 });
  });

  test('canvas preset changes dimensions and resets layout', async ({ page }) => {
    await page.click('[data-testid="preset-720p"]');
    const layout = await getLayoutState(page);
    // 720p = 1280x720, so half-sizes = 640x360
    expect(layout[0]).toMatchObject({ w: 640, h: 360 });
    expect(layout).toHaveLength(4);
  });

  test('rotated slot visual bounds stay within canvas', async ({ page }) => {
    const geo = await getCanvasGeometry(page);
    // Select slot 1 (top-left at 0,0, 960x540)
    const s1center = canvasToScreen(geo, 480, 270);
    await page.mouse.click(s1center.x, s1center.y);

    // Set rotation to 90 via the inspector
    await page.getByLabel('Rotation').selectOption('90');

    const layout = await getLayoutState(page);
    const slot1 = layout.find((s) => s.input === 'source:cam-1')!;
    expect(slot1.rotation).toBe(90);

    // Data model (x,y) = visual top-left of the axis-aligned bounding box.
    // For a 960x540 slot rotated 90°, visual dims are (vw=540, vh=960).
    const vw = slot1.h; // rotated 90°: visual width = data height
    const vh = slot1.w; // rotated 90°: visual height = data width

    expect(slot1.x).toBeGreaterThanOrEqual(0);
    expect(slot1.y).toBeGreaterThanOrEqual(0);
    expect(slot1.x + vw).toBeLessThanOrEqual(1920);
    expect(slot1.y + vh).toBeLessThanOrEqual(1080);
  });

  test('rotated slot drag stays within canvas bounds', async ({ page }) => {
    const geo = await getCanvasGeometry(page);

    // Remove slots 2-4
    for (const ref of ['source:cam-2', 'source:cam-3', 'source:cam-4']) {
      const layout = await getLayoutState(page);
      const slot = layout.find((s) => s.input === ref)!;
      const center = canvasToScreen(geo, slot.x + slot.w / 2, slot.y + slot.h / 2);
      await page.mouse.click(center.x, center.y);
      await page.click('[data-testid="remove-slot"]');
    }

    // Select slot 1 and rotate to 90
    const s1center = canvasToScreen(geo, 480, 270);
    await page.mouse.click(s1center.x, s1center.y);
    await page.getByLabel('Rotation').selectOption('90');

    // Slot 1 is now at (0,0) with visual dims (540, 960).
    // Try to drag it past the right edge of the 1920-wide canvas.
    // Visual center of the rotated slot: (0 + 540/2, 0 + 960/2) = (270, 480)
    await dragOnCanvas(page, geo, { cx: 270, cy: 480 }, { cx: 1800, cy: 480 });

    const layout = await getLayoutState(page);
    const slot1 = layout.find((s) => s.input === 'source:cam-1')!;
    const vw = slot1.h; // visual width for 90° rotation
    expect(slot1.x + vw).toBeLessThanOrEqual(1920);
  });

  test('resizing a rotated slot via inspector updates dimensions', async ({ page }) => {
    const geo = await getCanvasGeometry(page);

    // Remove all slots except slot 1
    for (const ref of ['source:cam-2', 'source:cam-3', 'source:cam-4']) {
      const layout = await getLayoutState(page);
      const slot = layout.find((s) => s.input === ref)!;
      const center = canvasToScreen(geo, slot.x + slot.w / 2, slot.y + slot.h / 2);
      await page.mouse.click(center.x, center.y);
      await page.click('[data-testid="remove-slot"]');
    }

    // Select slot 1 and set rotation to 90
    const s1center = canvasToScreen(geo, 480, 270);
    await page.mouse.click(s1center.x, s1center.y);
    await page.getByLabel('Rotation').selectOption('90');

    const before = await getLayoutState(page);
    const slotBefore = before.find((s) => s.input === 'source:cam-1')!;

    // Use the inspector W spinbutton (not checkbox labeled "W")
    const wInput = page.getByRole('spinbutton', { name: 'W' });
    await wInput.fill('800');
    await wInput.blur();

    const after = await getLayoutState(page);
    const slotAfter = after.find((s) => s.input === 'source:cam-1')!;
    expect(slotAfter.w).toBe(800);
    expect(slotAfter.h).toBe(slotBefore.h);
    expect(slotAfter.rotation).toBe(90);
  });

  test('changing aspect ratio mode updates layout state', async ({ page }) => {
    const geo = await getCanvasGeometry(page);
    // Select slot 1
    const s1center = canvasToScreen(geo, 480, 270);
    await page.mouse.click(s1center.x, s1center.y);

    // Change AR mode to fit
    await page.getByLabel('Aspect Ratio').selectOption('fit');

    const layout = await getLayoutState(page);
    const slot1 = layout.find((s) => s.input === 'source:cam-1')!;
    expect(slot1.aspect_ratio_mode).toBe('fit');

    // Change to crop
    await page.getByLabel('Aspect Ratio').selectOption('crop');
    const layout2 = await getLayoutState(page);
    const slot1b = layout2.find((s) => s.input === 'source:cam-1')!;
    expect(slot1b.aspect_ratio_mode).toBe('crop');
  });

  test('crop mode: shift+drag inside slot pans source offset', async ({ page }) => {
    const geo = await getCanvasGeometry(page);

    // Remove slots 1, 2, 3 to isolate slot 4 (640x480 = 4:3 source)
    for (const ref of ['source:cam-1', 'source:cam-2', 'source:cam-3']) {
      const layout = await getLayoutState(page);
      const slot = layout.find((s) => s.input === ref)!;
      const center = canvasToScreen(geo, slot.x + slot.w / 2, slot.y + slot.h / 2);
      await page.mouse.click(center.x, center.y);
      await page.click('[data-testid="remove-slot"]');
    }

    // Select slot 4, resize to ultra-wide to make crop dramatic
    const layout0 = await getLayoutState(page);
    const s4 = layout0.find((s) => s.input === 'source:cam-4')!;
    const s4c = canvasToScreen(geo, s4.x + s4.w / 2, s4.y + s4.h / 2);
    await page.mouse.click(s4c.x, s4c.y);

    // Set to crop mode
    await page.getByLabel('Aspect Ratio').selectOption('crop');

    // Resize to 960x200 via inspector to get a big AR mismatch
    await page.getByRole('spinbutton', { name: 'H' }).click();
    await page.getByRole('spinbutton', { name: 'H' }).fill('200');
    await page.getByRole('spinbutton', { name: 'Y', exact: true }).click(); // blur H

    const beforeDrag = await getLayoutState(page);
    const before = beforeDrag.find((s) => s.input === 'source:cam-4')!;
    const cropYBefore = before.crop_y ?? 0.5;

    // Shift+drag inside the slot to pan the source
    const slotCenter = canvasToScreen(geo, before.x + before.w / 2, before.y + before.h / 2);
    await page.keyboard.down('Shift');
    await page.mouse.move(slotCenter.x, slotCenter.y);
    await page.mouse.down();
    // Drag upward in screen = show lower part of source
    for (let i = 1; i <= 10; i++) {
      await page.mouse.move(slotCenter.x, slotCenter.y - i * 3);
    }
    await page.mouse.up();
    await page.keyboard.up('Shift');

    const afterDrag = await getLayoutState(page);
    const after = afterDrag.find((s) => s.input === 'source:cam-4')!;
    // crop_y should have changed from the drag
    expect(after.crop_y).not.toBe(cropYBefore);
    // Slot position should NOT have changed (shift+drag pans source, not slot)
    expect(after.x).toBe(before.x);
    expect(after.y).toBe(before.y);
  });

  test('crop mode: normal drag moves the slot', async ({ page }) => {
    const geo = await getCanvasGeometry(page);

    const s1center = canvasToScreen(geo, 480, 270);
    await page.mouse.click(s1center.x, s1center.y);
    await page.getByLabel('Aspect Ratio').selectOption('crop');

    const before = await getLayoutState(page);
    const slot1Before = before.find((s) => s.input === 'source:cam-1')!;

    await dragOnCanvas(page, geo,
      { cx: slot1Before.x + slot1Before.w / 2, cy: slot1Before.y + slot1Before.h / 2 },
      { cx: slot1Before.x + slot1Before.w / 2 + 50, cy: slot1Before.y + slot1Before.h / 2 },
    );

    const after = await getLayoutState(page);
    const slot1After = after.find((s) => s.input === 'source:cam-1')!;
    expect(slot1After.x).toBeGreaterThan(slot1Before.x);
  });

  test('crop mode: resizing output via edge handle changes w/h', async ({ page }) => {
    const geo = await getCanvasGeometry(page);

    // Remove all but slot 1
    for (const ref of ['source:cam-2', 'source:cam-3', 'source:cam-4']) {
      const layout = await getLayoutState(page);
      const slot = layout.find((s) => s.input === ref)!;
      const center = canvasToScreen(geo, slot.x + slot.w / 2, slot.y + slot.h / 2);
      await page.mouse.click(center.x, center.y);
      await page.click('[data-testid="remove-slot"]');
    }

    // Select slot 1, set crop
    const layout0 = await getLayoutState(page);
    const s1 = layout0[0]!;
    const s1c = canvasToScreen(geo, s1.x + s1.w / 2, s1.y + s1.h / 2);
    await page.mouse.click(s1c.x, s1c.y);
    await page.getByLabel('Aspect Ratio').selectOption('crop');

    const before = await getLayoutState(page);
    const slotBefore = before[0]!;

    // Drag the right edge handle leftward to shrink width
    const rightEdge = canvasToScreen(geo, slotBefore.x + slotBefore.w, slotBefore.y + slotBefore.h / 2);
    await page.mouse.move(rightEdge.x, rightEdge.y);
    await page.mouse.down();
    const target = canvasToScreen(geo, slotBefore.x + slotBefore.w - 200, slotBefore.y + slotBefore.h / 2);
    for (let i = 1; i <= 10; i++) {
      await page.mouse.move(
        rightEdge.x + ((target.x - rightEdge.x) * i) / 10,
        rightEdge.y + ((target.y - rightEdge.y) * i) / 10,
      );
    }
    await page.mouse.up();

    const after = await getLayoutState(page);
    const slotAfter = after[0]!;
    // Width should have decreased
    expect(slotAfter.w).toBeLessThan(slotBefore.w);
  });

  test('crop mode: resizing output via inspector changes w/h', async ({ page }) => {
    const geo = await getCanvasGeometry(page);
    // Select slot 1
    const s1c = canvasToScreen(geo, 480, 270);
    await page.mouse.click(s1c.x, s1c.y);
    await page.getByLabel('Aspect Ratio').selectOption('crop');

    const before = await getLayoutState(page);
    const slotBefore = before.find((s) => s.input === 'source:cam-1')!;

    // Resize via inspector — changes output w/h
    await page.getByRole('spinbutton', { name: 'W' }).click();
    await page.getByRole('spinbutton', { name: 'W' }).fill('800');
    await page.getByRole('spinbutton', { name: 'H' }).click(); // blur W

    const after = await getLayoutState(page);
    const slotAfter = after.find((s) => s.input === 'source:cam-1')!;
    expect(slotAfter.w).toBe(800);
    expect(slotAfter.h).toBe(slotBefore.h);
    expect(slotAfter.aspect_ratio_mode).toBe('crop');
  });

  test('crop mode: drag without shift moves slot position', async ({ page }) => {
    const geo = await getCanvasGeometry(page);

    const s1c = canvasToScreen(geo, 480, 270);
    await page.mouse.click(s1c.x, s1c.y);
    await page.getByLabel('Aspect Ratio').selectOption('crop');

    const before = await getLayoutState(page);
    const slotBefore = before.find((s) => s.input === 'source:cam-1')!;

    await dragOnCanvas(page, geo,
      { cx: slotBefore.x + slotBefore.w / 2, cy: slotBefore.y + slotBefore.h / 2 },
      { cx: slotBefore.x + slotBefore.w / 2 + 100, cy: slotBefore.y + slotBefore.h / 2 },
    );

    const after = await getLayoutState(page);
    const slotAfter = after.find((s) => s.input === 'source:cam-1')!;
    expect(slotAfter.x).toBeGreaterThan(slotBefore.x);
  });

  test('crop mode: dragging from crop band moves whole slot', async ({ page }) => {
    const geo = await getCanvasGeometry(page);

    // Isolate slot 4 (640x480 = 4:3 source)
    for (const ref of ['source:cam-1', 'source:cam-2', 'source:cam-3']) {
      const layout = await getLayoutState(page);
      const slot = layout.find((s) => s.input === ref)!;
      const center = canvasToScreen(geo, slot.x + slot.w / 2, slot.y + slot.h / 2);
      await page.mouse.click(center.x, center.y);
      await page.click('[data-testid="remove-slot"]');
    }

    // Select slot 4, resize to 960x200 for big AR mismatch, move to y=300
    const layout0 = await getLayoutState(page);
    const s4 = layout0.find((s) => s.input === 'source:cam-4')!;
    const s4c = canvasToScreen(geo, s4.x + s4.w / 2, s4.y + s4.h / 2);
    await page.mouse.click(s4c.x, s4c.y);
    await page.getByLabel('Aspect Ratio').selectOption('crop');
    await page.getByRole('spinbutton', { name: 'H' }).click();
    await page.getByRole('spinbutton', { name: 'H' }).fill('200');
    await page.getByRole('spinbutton', { name: 'Y', exact: true }).click();
    await page.getByRole('spinbutton', { name: 'Y', exact: true }).fill('300');
    await page.getByRole('spinbutton', { name: 'X', exact: true }).click(); // blur

    const before = await getLayoutState(page);
    const slotBefore = before.find((s) => s.input === 'source:cam-4')!;

    // Crop band extends above the output rect (fillScale=1.5, excessY=520, offset=-260).
    // Click at quarter-width to avoid the Transformer rotation anchor at top-center.
    const cropBandY = slotBefore.y - 100;
    const cx = slotBefore.x + slotBefore.w / 4;

    // Force hit canvas redraw after inspector changes
    await page.evaluate(() => {
      (window as any).Konva?.stages?.[0]?.children?.[0]?.draw();
    });

    await dragOnCanvas(page, geo,
      { cx, cy: cropBandY },
      { cx: cx - 80, cy: cropBandY },
    );

    const after = await getLayoutState(page);
    const slotAfter = after.find((s) => s.input === 'source:cam-4')!;
    // Normal drag from crop band moves the slot
    expect(slotAfter.x).toBeLessThan(slotBefore.x);
  });

  test('undo reverts last layout change', async ({ page }) => {
    const geo = await getCanvasGeometry(page);
    // Select slot 1 and nudge right
    const s1center = canvasToScreen(geo, 480, 270);
    await page.mouse.click(s1center.x, s1center.y);
    await page.keyboard.press('ArrowRight');

    let layout = await getLayoutState(page);
    expect(layout[0]!.x).toBe(10);

    // Undo via Ctrl+Z
    await page.keyboard.press('Control+z');
    layout = await getLayoutState(page);
    expect(layout[0]!.x).toBe(0);
  });

  test('redo restores undone change', async ({ page }) => {
    const geo = await getCanvasGeometry(page);
    const s1center = canvasToScreen(geo, 480, 270);
    await page.mouse.click(s1center.x, s1center.y);
    await page.keyboard.press('ArrowRight');

    let layout = await getLayoutState(page);
    expect(layout[0]!.x).toBe(10);

    await page.keyboard.press('Control+z');
    layout = await getLayoutState(page);
    expect(layout[0]!.x).toBe(0);

    // Redo via Ctrl+Shift+Z
    await page.keyboard.press('Control+Shift+z');
    layout = await getLayoutState(page);
    expect(layout[0]!.x).toBe(10);
  });

  test('crop mode: edge resize pins source to opposite edge', async ({ page }) => {
    const geo = await getCanvasGeometry(page);

    // Select slot 1 (0,0 960x540), set crop
    const s1c = canvasToScreen(geo, 480, 270);
    await page.mouse.click(s1c.x, s1c.y);
    await page.getByLabel('Aspect Ratio').selectOption('crop');

    const before = await getLayoutState(page);
    const slotBefore = before.find((s) => s.input === 'source:cam-1')!;
    const s2Before = before.find((s) => s.input === 'source:cam-2')!;
    const s3Before = before.find((s) => s.input === 'source:cam-3')!;

    // --- Left edge drag rightward ---
    const leftEdge = canvasToScreen(geo, slotBefore.x, slotBefore.y + slotBefore.h / 2);
    const leftTarget = canvasToScreen(geo, slotBefore.x + 300, slotBefore.y + slotBefore.h / 2);
    await page.mouse.move(leftEdge.x, leftEdge.y);
    await page.mouse.down();
    for (let i = 1; i <= 10; i++) {
      await page.mouse.move(
        leftEdge.x + ((leftTarget.x - leftEdge.x) * i) / 10,
        leftEdge.y,
      );
    }
    await page.mouse.up();

    const afterLeft = await getLayoutState(page);
    const slotAfterLeft = afterLeft.find((s) => s.input === 'source:cam-1')!;
    expect(slotAfterLeft.x).toBeGreaterThan(slotBefore.x);
    expect(slotAfterLeft.w).toBeLessThan(slotBefore.w);
    expect(slotAfterLeft.h).toBe(slotBefore.h);
    expect(slotAfterLeft.crop_x).toBe(1);

    // Other slots unchanged
    expect(afterLeft.find((s) => s.input === 'source:cam-2')!.x).toBe(s2Before.x);
    expect(afterLeft.find((s) => s.input === 'source:cam-3')!.x).toBe(s3Before.x);

    // --- Undo, then right edge drag leftward ---
    await page.click('[data-testid="undo"]');
    let reset = await getLayoutState(page);
    let slotReset = reset.find((s) => s.input === 'source:cam-1')!;
    expect(slotReset.x).toBe(slotBefore.x);

    const s1c3 = canvasToScreen(geo, slotReset.x + slotReset.w / 2, slotReset.y + slotReset.h / 2);
    await page.mouse.click(s1c3.x, s1c3.y);

    const rightEdge = canvasToScreen(geo, slotReset.x + slotReset.w, slotReset.y + slotReset.h / 2);
    const rightTarget = canvasToScreen(geo, slotReset.x + slotReset.w - 300, slotReset.y + slotReset.h / 2);
    await page.mouse.move(rightEdge.x, rightEdge.y);
    await page.mouse.down();
    for (let i = 1; i <= 10; i++) {
      await page.mouse.move(
        rightEdge.x + ((rightTarget.x - rightEdge.x) * i) / 10,
        rightEdge.y,
      );
    }
    await page.mouse.up();

    const afterRight = await getLayoutState(page);
    const slotAfterRight = afterRight.find((s) => s.input === 'source:cam-1')!;
    expect(slotAfterRight.x).toBe(slotBefore.x);
    expect(slotAfterRight.w).toBeLessThan(slotBefore.w);
    // Right edge drag: source pinned to LEFT (crop_x=0), crop band on right
    expect(slotAfterRight.crop_x).toBe(0);

    // --- Undo, then top edge drag downward ---
    await page.click('[data-testid="undo"]');
    reset = await getLayoutState(page);
    slotReset = reset.find((s) => s.input === 'source:cam-1')!;
    expect(slotReset.x).toBe(slotBefore.x);

    // Re-select slot 1
    const s1c2 = canvasToScreen(geo, slotReset.x + slotReset.w / 2, slotReset.y + slotReset.h / 2);
    await page.mouse.click(s1c2.x, s1c2.y);

    const topEdge = canvasToScreen(geo, slotReset.x + slotReset.w / 2, slotReset.y);
    const topTarget = canvasToScreen(geo, slotReset.x + slotReset.w / 2, slotReset.y + 200);
    await page.mouse.move(topEdge.x, topEdge.y);
    await page.mouse.down();
    for (let i = 1; i <= 10; i++) {
      await page.mouse.move(
        topEdge.x,
        topEdge.y + ((topTarget.y - topEdge.y) * i) / 10,
      );
    }
    await page.mouse.up();

    const afterTop = await getLayoutState(page);
    const slotAfterTop = afterTop.find((s) => s.input === 'source:cam-1')!;
    expect(slotAfterTop.y).toBeGreaterThan(slotBefore.y);
    expect(slotAfterTop.h).toBeLessThan(slotBefore.h);
    expect(slotAfterTop.w).toBe(slotBefore.w);
    expect(slotAfterTop.crop_y).toBe(1);
  });

  test('undo button works', async ({ page }) => {
    const geo = await getCanvasGeometry(page);
    const s1center = canvasToScreen(geo, 480, 270);
    await page.mouse.click(s1center.x, s1center.y);
    await page.keyboard.press('ArrowDown');

    let layout = await getLayoutState(page);
    expect(layout[0]!.y).toBe(10);

    await page.click('[data-testid="undo"]');
    layout = await getLayoutState(page);
    expect(layout[0]!.y).toBe(0);
  });
});
