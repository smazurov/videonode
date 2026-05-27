import { useEffect, useState } from 'react';
import { useShallow } from 'zustand/shallow';

import { KonvaCanvasEditor } from '../components/composers/KonvaCanvasEditor';
import type { SourceDims } from '../components/composers/KonvaCanvasEditor';
import { LayoutSlotInspector } from '../components/composers/LayoutSlotInspector';
import type {
  CanvasDims,
  ComposerInput,
  LayoutSlot,
} from '../lib/composer-types';
import { useLayoutEditorStore } from '../hooks/useLayoutEditorStore';

const PRESETS: Record<string, CanvasDims> = {
  '1080p': { w: 1920, h: 1080 },
  '720p': { w: 1280, h: 720 },
  '4K': { w: 3840, h: 2160 },
  'Square': { w: 1080, h: 1080 },
  'Vertical': { w: 1080, h: 1920 },
};

const SRC = 'source:cam-';

const MOCK_INPUTS: ComposerInput[] = [
  { ref: `${SRC}1` },
  { ref: `${SRC}2` },
  { ref: `${SRC}3`, effect: { type: 'perspective' } },
  { ref: `${SRC}4` },
];

const MOCK_SOURCE_DIMS = new Map<string, SourceDims>([
  [`${SRC}1`, { w: 1920, h: 1080 }],
  [`${SRC}2`, { w: 1280, h: 720 }],
  [`${SRC}3`, { w: 1080, h: 1920 }],
  [`${SRC}4`, { w: 640, h: 480 }],
]);

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

export default function TestLayoutEditor() {
  const store = useLayoutEditorStore;
  const { canvas, layout, selectedInput } = store(
    useShallow((s) => ({ canvas: s.canvas, layout: s.layout, selectedInput: s.selectedInput })),
  );
  const { setCanvas, setLayout, commitSlot, select, undo, redo, resetHistory } = store(
    useShallow((s) => ({
      setCanvas: s.setCanvas, setLayout: s.setLayout, commitSlot: s.commitSlot,
      select: s.select, undo: s.undo, redo: s.redo, resetHistory: s.resetHistory,
    })),
  );
  const canUndo = store((s) => s.past.length > 0);
  const canRedo = store((s) => s.future.length > 0);

  const [snapToGrid, setSnapToGrid] = useState(true);
  const [gridSize, setGridSize] = useState(10);
  const [showRulers, setShowRulers] = useState(true);
  const [showDebug, setShowDebug] = useState(true);

  // Initialize store on mount
  useEffect(() => {
    const initial = defaultLayout({ w: 1920, h: 1080 });
    resetHistory(initial);
    setCanvas({ w: 1920, h: 1080 });
    select(`${SRC}1`);
  }, [resetHistory, setCanvas, select]);

  // Ctrl+Z / Ctrl+Shift+Z
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!(e.metaKey || e.ctrlKey) || e.key.toLowerCase() !== 'z') return;
      const tag = (e.target as HTMLElement | null)?.tagName?.toLowerCase();
      if (tag === 'input' || tag === 'textarea') return;
      e.preventDefault();
      if (e.shiftKey) redo();
      else undo();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [undo, redo]);

  const selectedSlot = layout.find((s) => s.input === selectedInput) ?? null;
  const selectedSlotIndex = selectedInput
    ? layout.findIndex((s) => s.input === selectedInput)
    : -1;

  const onChangeSlot = (next: LayoutSlot) => {
    commitSlot(next.input, next);
  };

  const onChangeInputRef = (nextRef: string) => {
    if (!selectedInput) return;
    setLayout(layout.map((s) => (s.input === selectedInput ? { ...s, input: nextRef } : s)));
    select(nextRef);
  };

  const onBringToFront = () => {
    if (selectedSlotIndex < 0) return;
    const next = [...layout];
    const [moved] = next.splice(selectedSlotIndex, 1);
    if (moved) next.push(moved);
    setLayout(next);
  };

  const onSendToBack = () => {
    if (selectedSlotIndex < 0) return;
    const next = [...layout];
    const [moved] = next.splice(selectedSlotIndex, 1);
    if (moved) next.unshift(moved);
    setLayout(next);
  };

  const applyPreset = (key: string) => {
    const c = PRESETS[key];
    if (!c) return;
    setCanvas(c);
    resetHistory(defaultLayout(c));
  };

  const resetLayout = () => {
    resetHistory(defaultLayout(canvas));
    select(`${SRC}1`);
  };

  const addSlot = () => {
    const usedRefs = new Set(layout.map((s) => s.input));
    const available = MOCK_INPUTS.find((i) => !usedRefs.has(i.ref));
    if (!available) return;
    setLayout([...layout, { input: available.ref, x: 50, y: 50, w: 400, h: 300 }]);
  };

  const removeSelected = () => {
    if (!selectedInput) return;
    setLayout(layout.filter((s) => s.input !== selectedInput));
    select(null);
  };

  return (
    <div className="min-h-screen bg-bg text-fg" data-testid="test-layout-editor">
      <div className="border-b border-border bg-surface px-4 py-3">
        <div className="flex items-center justify-between">
          <h1 className="text-lg font-semibold">Layout Editor Test</h1>
          <div className="flex items-center gap-2 text-xs">
            <span className="text-fg-muted">
              Canvas: {canvas.w}x{canvas.h}
            </span>
            <span className="text-fg-muted">|</span>
            <span className="text-fg-muted">{layout.length} slots</span>
          </div>
        </div>
      </div>

      <div className="flex gap-4 p-4">
        <div className="w-72 shrink-0 space-y-4">
          <div className="rounded-md border border-border bg-surface p-3 space-y-2">
            <h3 className="text-xs font-semibold uppercase tracking-wide text-fg-muted">
              Canvas Preset
            </h3>
            <div className="flex flex-wrap gap-1">
              {Object.keys(PRESETS).map((key) => (
                <button
                  key={key}
                  type="button"
                  onClick={() => applyPreset(key)}
                  className={`rounded px-2 py-1 text-xs ${
                    PRESETS[key]!.w === canvas.w && PRESETS[key]!.h === canvas.h
                      ? 'bg-accent-soft text-accent-soft-fg'
                      : 'bg-surface-muted text-fg-muted hover:bg-surface-raised'
                  }`}
                  data-testid={`preset-${key}`}
                >
                  {key}
                </button>
              ))}
            </div>
          </div>

          <div className="rounded-md border border-border bg-surface p-3 space-y-2">
            <h3 className="text-xs font-semibold uppercase tracking-wide text-fg-muted">
              Grid
            </h3>
            <label className="flex items-center gap-2 text-xs text-fg-muted">
              <input
                type="checkbox"
                checked={snapToGrid}
                onChange={(e) => setSnapToGrid(e.target.checked)}
                data-testid="snap-toggle"
              />
              Snap to grid
            </label>
            <label className="flex items-center gap-2 text-xs text-fg-muted">
              Grid size:
              <input
                type="number"
                min={1}
                max={200}
                value={gridSize}
                onChange={(e) => {
                  const v = Number.parseInt(e.target.value, 10);
                  if (!Number.isNaN(v) && v > 0) setGridSize(v);
                }}
                className="w-16 rounded-sm border border-border-strong bg-surface px-2 py-0.5 text-xs text-fg"
                data-testid="grid-size"
              />
            </label>
            <label className="flex items-center gap-2 text-xs text-fg-muted">
              <input
                type="checkbox"
                checked={showRulers}
                onChange={(e) => setShowRulers(e.target.checked)}
                data-testid="rulers-toggle"
              />
              Show rulers
            </label>
          </div>

          <div className="rounded-md border border-border bg-surface p-3 space-y-2">
            <h3 className="text-xs font-semibold uppercase tracking-wide text-fg-muted">
              Actions
            </h3>
            <div className="flex flex-wrap gap-1">
              <button
                type="button"
                onClick={resetLayout}
                className="rounded bg-surface-muted px-2 py-1 text-xs text-fg-muted hover:bg-surface-raised"
                data-testid="reset-layout"
              >
                Reset
              </button>
              <button
                type="button"
                onClick={addSlot}
                disabled={layout.length >= MOCK_INPUTS.length}
                className="rounded bg-surface-muted px-2 py-1 text-xs text-fg-muted hover:bg-surface-raised disabled:opacity-50"
                data-testid="add-slot"
              >
                Add slot
              </button>
              <button
                type="button"
                onClick={undo}
                disabled={!canUndo}
                className="rounded bg-surface-muted px-2 py-1 text-xs text-fg-muted hover:bg-surface-raised disabled:opacity-50"
                data-testid="undo"
              >
                Undo
              </button>
              <button
                type="button"
                onClick={redo}
                disabled={!canRedo}
                className="rounded bg-surface-muted px-2 py-1 text-xs text-fg-muted hover:bg-surface-raised disabled:opacity-50"
                data-testid="redo"
              >
                Redo
              </button>
            </div>
          </div>

          <LayoutSlotInspector
            slot={selectedSlot}
            canvas={canvas}
            inputs={MOCK_INPUTS}
            slotIndex={selectedSlotIndex}
            layoutLength={layout.length}
            onChange={onChangeSlot}
            onChangeInputRef={onChangeInputRef}
            onBringToFront={onBringToFront}
            onSendToBack={onSendToBack}
            {...(selectedInput ? { onDelete: removeSelected } : {})}
          />

          {showDebug && (
            <div className="rounded-md border border-border bg-surface p-3 space-y-2">
              <div className="flex items-center justify-between">
                <h3 className="text-xs font-semibold uppercase tracking-wide text-fg-muted">
                  Layout State
                </h3>
                <button
                  type="button"
                  onClick={() => setShowDebug(false)}
                  className="text-xs text-fg-subtle hover:text-fg-muted"
                >
                  hide
                </button>
              </div>
              <pre
                className="max-h-60 overflow-auto whitespace-pre-wrap text-[10px] text-fg-subtle font-mono"
                data-testid="layout-debug"
              >
                {JSON.stringify(layout, null, 2)}
              </pre>
            </div>
          )}
        </div>

        <div className="flex-1 min-w-0" data-testid="canvas-container">
          <KonvaCanvasEditor
            inputs={MOCK_INPUTS}
            sourceDims={MOCK_SOURCE_DIMS}
            gridSize={gridSize}
            snapToGrid={snapToGrid}
            showRulers={showRulers}
          />
        </div>
      </div>
    </div>
  );
}
