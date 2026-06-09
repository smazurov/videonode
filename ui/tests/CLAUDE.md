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

### Transformer `transformend` does not bubble
Konva's `transformend` event fires only on the node attached to the Transformer — it does NOT bubble to parent Groups. Place `onTransformEnd` directly on the Rect the Transformer targets, not on a parent Group.

### Canvas bounds clamping
Slots are clamped to `[0, canvasW - slotW]` on both axes. A slot at x=960 with w=960 on a 1920-wide canvas is at the maximum x — dragging right produces no visible change. Always drag toward available space.

### Coordinate conversion
Canvas coords to screen: `screenX = stageX + canvasX * scale`. The stage scale is `containerWidth / canvasWidth`.
