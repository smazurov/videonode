import { useState } from 'react';
import {
  ArrowUturnLeftIcon,
  ArrowUturnRightIcon,
  TrashIcon,
} from '@heroicons/react/24/outline';

import { Button } from '../Button';
import { InputField } from '../InputField';
import { Select } from '../Select';
import type { AspectRatioMode, CanvasDims, ComposerInput, LayoutSlot } from '../../lib/composer-types';

interface LayoutSlotInspectorProps {
  slot: LayoutSlot | null;
  canvas: CanvasDims;
  inputs: readonly ComposerInput[];
  /** Index of `slot` within the parent layout array; used by z-order actions. */
  slotIndex: number;
  layoutLength: number;
  onChange: (next: LayoutSlot) => void;
  onChangeInputRef: (nextRef: string) => void;
  onBringToFront: () => void;
  onSendToBack: () => void;
  onDelete?: () => void;
}

// Right-pane editor for the currently selected slot. Numeric inputs commit
// onBlur (avoids saving every keystroke; CanvasEditor handles drag debounce).
export function LayoutSlotInspector({
  slot,
  canvas,
  inputs,
  slotIndex,
  layoutLength,
  onChange,
  onChangeInputRef,
  onBringToFront,
  onSendToBack,
  onDelete,
}: Readonly<LayoutSlotInspectorProps>) {
  // Track in-progress edits only while an input is focused.
  // When not editing, values come directly from the slot prop (zero-delay sync).
  const [editing, setEditing] = useState<Record<string, string>>({});

  if (!slot) {
    return (
      <div className="rounded-md border border-border bg-surface p-4 text-sm text-fg-subtle">
        Select a slot to edit its placement.
      </div>
    );
  }

  type NumKey = 'x' | 'y' | 'w' | 'h';

  const numVal = (key: NumKey) =>
    key in editing ? editing[key]! : String(slot[key]);

  const clampNum = (key: NumKey, v: number) => {
    if (key === 'w') return Math.max(1, Math.min(canvas.w, v));
    if (key === 'h') return Math.max(1, Math.min(canvas.h, v));
    if (key === 'x') return Math.max(0, Math.min(canvas.w - slot.w, v));
    return Math.max(0, Math.min(canvas.h - slot.h, v));
  };

  const onNumFocus = (key: string) =>
    setEditing((e) => ({ ...e, [key]: String(slot[key as keyof LayoutSlot]) }));

  const onNumChange = (key: NumKey, raw: string) => {
    setEditing((e) => ({ ...e, [key]: raw }));
    const parsed = Number.parseInt(raw, 10);
    if (!Number.isNaN(parsed)) onChange({ ...slot, [key]: clampNum(key, parsed) });
  };

  const onNumBlur = (key: NumKey) => {
    setEditing((e) => { const r = { ...e }; delete r[key]; return r; });
  };

  return (
    <div className="rounded-md border border-border bg-surface p-4 space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-semibold text-fg">Slot</h3>
      </div>

      <Select
        label="Input"
        value={slot.input}
        onChange={(e) => onChangeInputRef(e.target.value)}
      >
        {inputs.map((input) => (
          <option key={input.ref} value={input.ref}>
            {input.ref}
            {input.effect ? ` · ${input.effect.type}` : ''}
          </option>
        ))}
      </Select>

      <div className="grid grid-cols-2 gap-3">
        <InputField
          label="X"
          type="number"
          value={numVal('x')}
          min={0}
          max={canvas.w}
          onFocus={() => onNumFocus('x')}
          onChange={(e) => onNumChange('x', e.target.value)}
          onBlur={() => onNumBlur('x')}
        />
        <InputField
          label="Y"
          type="number"
          value={numVal('y')}
          min={0}
          max={canvas.h}
          onFocus={() => onNumFocus('y')}
          onChange={(e) => onNumChange('y', e.target.value)}
          onBlur={() => onNumBlur('y')}
        />
        <InputField
          label="W"
          type="number"
          value={numVal('w')}
          min={1}
          max={canvas.w}
          onFocus={() => onNumFocus('w')}
          onChange={(e) => onNumChange('w', e.target.value)}
          onBlur={() => onNumBlur('w')}
        />
        <InputField
          label="H"
          type="number"
          value={numVal('h')}
          min={1}
          max={canvas.h}
          onFocus={() => onNumFocus('h')}
          onChange={(e) => onNumChange('h', e.target.value)}
          onBlur={() => onNumBlur('h')}
        />
      </div>

      <div className="space-y-1">
        <label className="block text-sm font-medium text-fg">Rotation</label>
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => {
              const next = ((((slot.rotation ?? 0) - 90) % 360) + 360) % 360;
              onChange({ ...slot, rotation: next });
            }}
            title="Rotate 90° counter-clockwise"
            aria-label="Rotate 90° counter-clockwise"
            className="rounded-sm p-1.5 text-fg-muted hover:bg-surface-muted"
          >
            <ArrowUturnLeftIcon className="h-4 w-4" />
          </button>
          <div className="flex-1">
            <Select
              value={String(slot.rotation ?? 0)}
              onChange={(e) => {
                onChange({ ...slot, rotation: Number.parseInt(e.target.value, 10) });
              }}
            >
              <option value="0">0</option>
              <option value="90">90</option>
              <option value="180">180</option>
              <option value="270">270</option>
            </Select>
          </div>
          <button
            type="button"
            onClick={() => {
              const next = (((slot.rotation ?? 0) + 90) % 360 + 360) % 360;
              onChange({ ...slot, rotation: next });
            }}
            title="Rotate 90° clockwise"
            aria-label="Rotate 90° clockwise"
            className="rounded-sm p-1.5 text-fg-muted hover:bg-surface-muted"
          >
            <ArrowUturnRightIcon className="h-4 w-4" />
          </button>
        </div>
      </div>

      <Select
        label="Aspect Ratio"
        value={slot.aspect_ratio_mode ?? 'stretch'}
        onChange={(e) => {
          onChange({ ...slot, aspect_ratio_mode: e.target.value as AspectRatioMode });
        }}
      >
        <option value="stretch">Stretch</option>
        <option value="fit">Fit (letterbox)</option>
        <option value="crop">Crop (fill)</option>
      </Select>

      {slot.aspect_ratio_mode === 'crop' && (
        <div className="grid grid-cols-2 gap-3">
          <InputField
            label="Crop X"
            type="number"
            value={'crop_x' in editing ? editing.crop_x! : String(Math.round((slot.crop?.x ?? 0.5) * 100))}
            min={0}
            max={100}
            onFocus={() => setEditing((e) => ({ ...e, crop_x: String(Math.round((slot.crop?.x ?? 0.5) * 100)) }))}
            onChange={(e) => {
              setEditing((ed) => ({ ...ed, crop_x: e.target.value }));
              const v = Number.parseInt(e.target.value, 10);
              if (!Number.isNaN(v)) onChange({ ...slot, crop: { ...slot.crop ?? { x: 0.5, y: 0.5, scale: 1 }, x: Math.max(0, Math.min(100, v)) / 100 } });
            }}
            onBlur={() => setEditing((e) => { const r = { ...e }; delete r.crop_x; return r; })}
          />
          <InputField
            label="Crop Y"
            type="number"
            value={'crop_y' in editing ? editing.crop_y! : String(Math.round((slot.crop?.y ?? 0.5) * 100))}
            min={0}
            max={100}
            onFocus={() => setEditing((e) => ({ ...e, crop_y: String(Math.round((slot.crop?.y ?? 0.5) * 100)) }))}
            onChange={(e) => {
              setEditing((ed) => ({ ...ed, crop_y: e.target.value }));
              const v = Number.parseInt(e.target.value, 10);
              if (!Number.isNaN(v)) onChange({ ...slot, crop: { ...slot.crop ?? { x: 0.5, y: 0.5, scale: 1 }, y: Math.max(0, Math.min(100, v)) / 100 } });
            }}
            onBlur={() => setEditing((e) => { const r = { ...e }; delete r.crop_y; return r; })}
          />
        </div>
      )}

      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          size="SM"
          theme="light"
          text="Send to back"
          onClick={onSendToBack}
          disabled={slotIndex === 0}
        />
        <Button
          type="button"
          size="SM"
          theme="light"
          text="Bring to front"
          onClick={onBringToFront}
          disabled={slotIndex === layoutLength - 1}
        />
        {onDelete && (
          <button
            type="button"
            onClick={onDelete}
            data-testid="remove-slot"
            title="Remove slot"
            className="rounded-sm p-1.5 text-danger hover:bg-danger/10"
          >
            <TrashIcon className="h-4 w-4" />
          </button>
        )}
      </div>
    </div>
  );
}
