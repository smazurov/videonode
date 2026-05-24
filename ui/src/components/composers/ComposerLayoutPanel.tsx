import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import toast from 'react-hot-toast';

import { Card } from '../Card';
import { Checkbox } from '../Checkbox';
import { InputField } from '../InputField';
import { CanvasEditor } from './CanvasEditor';
import { LayoutSlotInspector } from './LayoutSlotInspector';
import type { ComposerData, LayoutSlot } from '../../lib/composer-types';
import { useComposerStore } from '../../hooks/useComposerStore';

interface ComposerLayoutPanelProps {
  composer: ComposerData;
}

const SAVE_DEBOUNCE_MS = 250;

// Interactive composer layout editor. Drag-resizable slots write through
// useComposerStore.updateComposerLayout with a 250ms debounce; the
// optimistic local copy rolls back when the PATCH rejects.
export function ComposerLayoutPanel({ composer }: Readonly<ComposerLayoutPanelProps>) {
  const updateComposerLayout = useComposerStore((s) => s.updateComposerLayout);

  const [localLayout, setLocalLayout] = useState<LayoutSlot[]>(composer.layout);
  const [selectedInput, setSelectedInput] = useState<string | null>(
    composer.layout[0]?.input ?? null,
  );
  const [snapToGrid, setSnapToGrid] = useState(true);
  const [gridSize, setGridSize] = useState(10);
  const [showRulers, setShowRulers] = useState(true);
  const [saving, setSaving] = useState(false);

  const persistedRef = useRef<LayoutSlot[]>(composer.layout);
  const saveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const pendingRef = useRef<LayoutSlot[] | null>(null);

  // Reseed local state when the upstream composer changes from
  // somewhere other than this editor (SSE event, another tab).
  useEffect(() => {
    persistedRef.current = composer.layout;
    setLocalLayout(composer.layout);
    if (composer.layout.length > 0 && !selectedInput) {
      setSelectedInput(composer.layout[0]?.input ?? null);
    }
  }, [composer.layout, selectedInput]);

  const rollback = useCallback(() => {
    setLocalLayout(persistedRef.current);
  }, []);

  const runSave = useCallback(async () => {
    const toSave = pendingRef.current;
    if (!toSave) return;
    setSaving(true);
    try {
      await updateComposerLayout(composer.composer_id, toSave);
      persistedRef.current = toSave;
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Save failed');
      rollback();
    } finally {
      setSaving(false);
    }
  }, [composer.composer_id, rollback, updateComposerLayout]);

  const scheduleSave = useCallback(
    (layout: LayoutSlot[]) => {
      pendingRef.current = layout;
      if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
      saveTimerRef.current = setTimeout(() => void runSave(), SAVE_DEBOUNCE_MS);
    },
    [runSave],
  );

  useEffect(() => {
    return () => {
      if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
    };
  }, []);

  const handleLayoutChange = useCallback(
    (next: LayoutSlot[]) => {
      setLocalLayout(next);
      scheduleSave(next);
    },
    [scheduleSave],
  );

  const selectedSlot = useMemo(() => {
    if (!selectedInput) return null;
    return localLayout.find((s) => s.input === selectedInput) ?? null;
  }, [localLayout, selectedInput]);

  const selectedSlotIndex = useMemo(() => {
    if (!selectedInput) return -1;
    return localLayout.findIndex((s) => s.input === selectedInput);
  }, [localLayout, selectedInput]);

  const onChangeSlot = useCallback(
    (next: LayoutSlot) => {
      const nextLayout = localLayout.map((s) => (s.input === next.input ? next : s));
      handleLayoutChange(nextLayout);
    },
    [handleLayoutChange, localLayout],
  );

  const onChangeInputRef = useCallback(
    (nextRef: string) => {
      if (!selectedInput) return;
      const nextLayout = localLayout.map((s) =>
        s.input === selectedInput ? { ...s, input: nextRef } : s,
      );
      setSelectedInput(nextRef);
      handleLayoutChange(nextLayout);
    },
    [handleLayoutChange, localLayout, selectedInput],
  );

  const onBringToFront = useCallback(() => {
    if (selectedSlotIndex < 0) return;
    const next = [...localLayout];
    const [moved] = next.splice(selectedSlotIndex, 1);
    if (moved) next.push(moved);
    handleLayoutChange(next);
  }, [handleLayoutChange, localLayout, selectedSlotIndex]);

  const onSendToBack = useCallback(() => {
    if (selectedSlotIndex < 0) return;
    const next = [...localLayout];
    const [moved] = next.splice(selectedSlotIndex, 1);
    if (moved) next.unshift(moved);
    handleLayoutChange(next);
  }, [handleLayoutChange, localLayout, selectedSlotIndex]);

  return (
    <Card padding="none">
      <div className="flex items-center justify-between border-b border-border px-4 py-3">
        <h2 className="text-sm font-semibold text-fg">Layout</h2>
        {saving ? (
          <span className="text-xs text-fg-subtle">saving…</span>
        ) : (
          <span className="text-xs text-fg-muted">{localLayout.length} slot{localLayout.length === 1 ? '' : 's'}</span>
        )}
      </div>

      <div className="grid grid-cols-1 gap-4 p-4 lg:grid-cols-[1fr_320px]">
        <div className="space-y-3">
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
          <CanvasEditor
            canvas={composer.canvas}
            inputs={composer.inputs}
            layout={localLayout}
            selectedInput={selectedInput}
            onSelect={setSelectedInput}
            onLayoutChange={handleLayoutChange}
            gridSize={gridSize}
            snapToGrid={snapToGrid}
            showRulers={showRulers}
            saving={saving}
          />
        </div>
        <div className="space-y-3">
          <LayoutSlotInspector
            slot={selectedSlot}
            canvas={composer.canvas}
            inputs={composer.inputs}
            slotIndex={selectedSlotIndex}
            layoutLength={localLayout.length}
            onChange={onChangeSlot}
            onChangeInputRef={onChangeInputRef}
            onBringToFront={onBringToFront}
            onSendToBack={onSendToBack}
            saving={saving}
          />
          {localLayout.length > 0 && (
            <div className="rounded-md border border-border bg-surface p-3">
              <h4 className="mb-2 text-xs font-semibold uppercase tracking-wide text-fg-muted">
                Slots
              </h4>
              <ul className="space-y-1">
                {localLayout.map((slot) => (
                  <li key={slot.input}>
                    <button
                      type="button"
                      onClick={() => setSelectedInput(slot.input)}
                      className={`w-full rounded-sm px-2 py-1 text-left font-mono text-xs ${
                        slot.input === selectedInput
                          ? 'bg-accent-soft text-accent-soft-fg'
                          : 'text-fg-muted hover:bg-surface-muted'
                      }`}
                    >
                      {slot.input}
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
