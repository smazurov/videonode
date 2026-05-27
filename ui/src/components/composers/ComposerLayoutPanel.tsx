import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import toast from 'react-hot-toast';
import { useShallow } from 'zustand/shallow';

import { Card } from '../Card';
import { Checkbox } from '../Checkbox';
import { InputField } from '../InputField';
import { KonvaCanvasEditor } from './KonvaCanvasEditor';
import type { SourceDims } from './KonvaCanvasEditor';
import { LayoutSlotInspector } from './LayoutSlotInspector';
import type { ComposerData, LayoutSlot } from '../../lib/composer-types';
import { useComposerStore } from '../../hooks/useComposerStore';
import { useLayoutEditorStore } from '../../hooks/useLayoutEditorStore';
import { useSourceStore } from '../../hooks/useSourceStore';

interface ComposerLayoutPanelProps {
  composer: ComposerData;
}

const SAVE_DEBOUNCE_MS = 250;

export function ComposerLayoutPanel({ composer }: Readonly<ComposerLayoutPanelProps>) {
  const updateComposerLayout = useComposerStore((s) => s.updateComposerLayout);
  const store = useLayoutEditorStore;
  const { layout, selectedInput } = store(
    useShallow((s) => ({ layout: s.layout, selectedInput: s.selectedInput })),
  );
  const { setCanvas, setLayout, select, resetHistory } = store(
    useShallow((s) => ({ setCanvas: s.setCanvas, setLayout: s.setLayout, select: s.select, resetHistory: s.resetHistory })),
  );

  const sourcesById = useSourceStore((s) => s.sourcesById);
  const sourceDims = useMemo(() => {
    const m = new Map<string, SourceDims>();
    for (const inp of composer.inputs) {
      const id = inp.ref.replace(/^source:/, '');
      const src = sourcesById[id];
      const live = src?.latest_status?.format;
      const cfg = src?.format;
      const w = live?.w ?? cfg?.width;
      const h = live?.h ?? cfg?.height;
      if (w && h) m.set(inp.ref, { w, h });
    }
    return m;
  }, [composer.inputs, sourcesById]);

  const [snapToGrid, setSnapToGrid] = useState(true);
  const [gridSize, setGridSize] = useState(10);
  const [showRulers, setShowRulers] = useState(true);
  const [saving, setSaving] = useState(false);

  const saveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const pendingRef = useRef<LayoutSlot[] | null>(null);
  const saveGenRef = useRef(0);
  const saveInFlightRef = useRef(false);
  const persistedJsonRef = useRef(JSON.stringify(composer.layout));

  // Sync store from composer data (init + upstream reseed)
  useEffect(() => {
    if (saveInFlightRef.current) {
      persistedJsonRef.current = JSON.stringify(composer.layout);
      return;
    }
    persistedJsonRef.current = JSON.stringify(composer.layout);
    setCanvas(composer.canvas);
    resetHistory(composer.layout);
    if (composer.layout.length > 0) {
      select(composer.layout[0]?.input ?? null);
    }
  }, [composer.composer_id, composer.canvas, composer.layout, resetHistory, setCanvas, select]);

  const rollback = useCallback(() => {
    resetHistory(JSON.parse(persistedJsonRef.current) as LayoutSlot[]);
  }, [resetHistory]);

  const runSave = useCallback(async () => {
    const toSave = pendingRef.current;
    if (!toSave) return;
    const gen = ++saveGenRef.current;
    setSaving(true);
    saveInFlightRef.current = true;
    try {
      await updateComposerLayout(composer.composer_id, toSave);
      if (saveGenRef.current === gen) {
        persistedJsonRef.current = JSON.stringify(toSave);
      }
    } catch (error) {
      if (saveGenRef.current === gen) {
        toast.error(error instanceof Error ? error.message : 'Save failed');
        rollback();
      }
    } finally {
      if (saveGenRef.current === gen) {
        setSaving(false);
        saveInFlightRef.current = false;
      }
    }
  }, [composer.composer_id, rollback, updateComposerLayout]);

  const scheduleSave = useCallback(
    (l: LayoutSlot[]) => {
      pendingRef.current = l;
      if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
      saveTimerRef.current = setTimeout(() => void runSave(), SAVE_DEBOUNCE_MS);
    },
    [runSave],
  );

  // Subscribe to store layout changes for debounced save.
  // Skip when layout content matches persisted (init, reseed, save echo).
  useEffect(() => {
    return useLayoutEditorStore.subscribe(
      (s) => s.layout,
      (next) => {
        if (JSON.stringify(next) === persistedJsonRef.current) return;
        scheduleSave(next);
      },
    );
  }, [scheduleSave]);

  useEffect(() => {
    return () => {
      if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
    };
  }, []);

  const selectedSlot = useMemo(() => {
    if (!selectedInput) return null;
    return layout.find((s) => s.input === selectedInput) ?? null;
  }, [layout, selectedInput]);

  const selectedSlotIndex = useMemo(() => {
    if (!selectedInput) return -1;
    return layout.findIndex((s) => s.input === selectedInput);
  }, [layout, selectedInput]);

  const onChangeSlot = useCallback(
    (next: LayoutSlot) => {
      setLayout(layout.map((s) => (s.input === next.input ? next : s)));
    },
    [layout, setLayout],
  );

  const onChangeInputRef = useCallback(
    (nextRef: string) => {
      if (!selectedInput) return;
      setLayout(layout.map((s) => s.input === selectedInput ? { ...s, input: nextRef } : s));
      select(nextRef);
    },
    [layout, selectedInput, select, setLayout],
  );

  const onBringToFront = useCallback(() => {
    if (selectedSlotIndex < 0) return;
    const next = [...layout];
    const [moved] = next.splice(selectedSlotIndex, 1);
    if (moved) next.push(moved);
    setLayout(next);
  }, [layout, selectedSlotIndex, setLayout]);

  const onSendToBack = useCallback(() => {
    if (selectedSlotIndex < 0) return;
    const next = [...layout];
    const [moved] = next.splice(selectedSlotIndex, 1);
    if (moved) next.unshift(moved);
    setLayout(next);
  }, [layout, selectedSlotIndex, setLayout]);

  return (
    <Card padding="none">
      <div className="flex items-center justify-between border-b border-border px-4 py-3">
        <h2 className="text-sm font-semibold text-fg">Layout</h2>
        <span className="text-xs text-fg-muted">
          {saving && <span className="mr-2 text-fg-subtle">saving… ·</span>}
          {layout.length} slot{layout.length === 1 ? '' : 's'}
        </span>
      </div>

      <div className="grid grid-cols-1 gap-4 p-4 lg:grid-cols-[minmax(0,1fr)_320px]">
        <div className="min-w-0 space-y-3">
          <div className="flex flex-wrap items-center gap-4 text-xs text-fg-muted">
            <Checkbox
              label="Snap to grid"
              checked={snapToGrid}
              onChange={(e) => setSnapToGrid(e.target.checked)}
            />
            <div className="flex items-center gap-2">
              <label htmlFor="grid-size" className="text-fg-muted">
                Grid
              </label>
              <InputField
                id="grid-size"
                type="number"
                min={1}
                max={200}
                value={gridSize}
                onChange={(e) => {
                  const v = Number.parseInt(e.target.value, 10);
                  if (!Number.isNaN(v) && v > 0) setGridSize(v);
                }}
                className="w-20"
              />
            </div>
            <Checkbox
              label="Rulers"
              checked={showRulers}
              onChange={(e) => setShowRulers(e.target.checked)}
            />
          </div>
          <KonvaCanvasEditor
            inputs={composer.inputs}
            sourceDims={sourceDims}
            gridSize={gridSize}
            snapToGrid={snapToGrid}
            showRulers={showRulers}
          />
        </div>
        <div className="space-y-3">
          <LayoutSlotInspector
            slot={selectedSlot}
            canvas={composer.canvas}
            inputs={composer.inputs}
            slotIndex={selectedSlotIndex}
            layoutLength={layout.length}
            onChange={onChangeSlot}
            onChangeInputRef={onChangeInputRef}
            onBringToFront={onBringToFront}
            onSendToBack={onSendToBack}
          />
          {layout.length > 0 && (
            <div className="rounded-md border border-border bg-surface p-3">
              <h4 className="mb-2 text-xs font-semibold uppercase tracking-wide text-fg-muted">
                Slots
              </h4>
              <ul className="space-y-1">
                {layout.map((slot, i) => (
                  <li key={slot.input}>
                    <button
                      type="button"
                      onClick={() => select(slot.input)}
                      title={slot.input}
                      className={`w-full truncate rounded-sm px-2 py-1 text-left font-mono text-xs ${
                        slot.input === selectedInput
                          ? 'bg-accent-soft text-accent-soft-fg'
                          : 'text-fg-muted hover:bg-surface-muted'
                      }`}
                    >
                      {i + 1}. {slot.input}
                    </button>
                  </li>
                ))}
              </ul>
            </div>
          )}
        </div>
      </div>
    </Card>
  );
}
