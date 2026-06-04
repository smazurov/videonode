# Playwright Tests

## Running Tests

```bash
cd ui && npx playwright test --reporter=list
```

Run a single test by name:
```bash
npx playwright test --reporter=list -g "crop mode: resizing output"
```

`playwright-cli` is available globally for interactive browser automation.

## Konva Canvas Testing Pitfalls

### Hit canvas staleness
After modifying Konva node properties via React (e.g., inspector input changes), the hit canvas is NOT automatically redrawn. Konva's `batchDraw()` only schedules a redraw on the next animation frame. If Playwright fires mouse events before that frame, `getIntersection()` returns stale results.

Fix: force a redraw before interacting:
```typescript
await page.evaluate(() => {
  (window as any).Konva?.stages?.[0]?.children?.[0]?.draw();
});
```

### Transformer rotation anchor occlusion
The Konva Transformer places a 10x10 rotation anchor 50px above the top-center of the attached node. In crop mode, this anchor sits in the crop band area. Clicking at the top-center of the crop band hits the rotation anchor instead of the dim band Rect.

Fix: offset the click x-position away from center (e.g., quarter-width).

### Transformer `transformend` does not bubble
Konva's `transformend` event fires only on the node attached to the Transformer — it does NOT bubble to parent Groups. Place `onTransformEnd` directly on the Rect the Transformer targets, not on a parent Group.

### Canvas bounds clamping
Slots are clamped to `[0, canvasW - slotW]` on both axes. A slot at x=960 with w=960 on a 1920-wide canvas is at the maximum x — dragging right produces no visible change. Always drag toward available space.

### Coordinate conversion
Canvas coords to screen: `screenX = stageX + canvasX * scale`. The `canvasToScreen` and `getCanvasGeometry` helpers handle this. The stage scale is `containerWidth / canvasWidth`.
