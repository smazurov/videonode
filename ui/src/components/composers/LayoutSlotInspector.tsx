import { useEffect, useState } from 'react';

import { Button } from '../Button';
import { InputField } from '../InputField';
import { Select } from '../Select';
import type { CanvasDims, ComposerInput, LayoutSlot } from '../../lib/composer-types';

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
  const [local, setLocal] = useState<LayoutSlot | null>(slot);

  // Sync local state when the selection changes or upstream layout updates.
  useEffect(() => {
    setLocal(slot);
  }, [slot]);

  if (!slot || !local) {
    return (
      <div className="rounded-md border border-border bg-surface p-4 text-sm text-fg-subtle">
        Select a slot to edit its placement.
      </div>
    );
  }

  const commit = (next: LayoutSlot) => {
    setLocal(next);
    onChange(next);
  };

  const onNumChange = (key: 'x' | 'y' | 'w' | 'h', raw: string) => {
    const num = Number.parseInt(raw, 10);
    if (Number.isNaN(num)) return;
    const next = { ...local, [key]: num };
    setLocal(next);
  };

  const onNumBlur = (key: 'x' | 'y' | 'w' | 'h') => {
    let v = local[key];
    if (key === 'w') v = Math.max(1, Math.min(canvas.w, v));
    else if (key === 'h') v = Math.max(1, Math.min(canvas.h, v));
    else if (key === 'x') v = Math.max(0, Math.min(canvas.w - local.w, v));
    else if (key === 'y') v = Math.max(0, Math.min(canvas.h - local.h, v));
    const next = { ...local, [key]: v };
    commit(next);
  };

  return (
    <div className="rounded-md border border-border bg-surface p-4 space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-semibold text-fg">Slot</h3>
      </div>

      <Select
        label="Input"
        value={local.input}
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
          value={local.x}
          min={0}
          max={canvas.w}
          onChange={(e) => onNumChange('x', e.target.value)}
          onBlur={() => onNumBlur('x')}
/>
        <InputField
          label="Y"
          type="number"
          value={local.y}
          min={0}
          max={canvas.h}
          onChange={(e) => onNumChange('y', e.target.value)}
          onBlur={() => onNumBlur('y')}
/>
        <InputField
          label="W"
          type="number"
          value={local.w}
          min={1}
          max={canvas.w}
          onChange={(e) => onNumChange('w', e.target.value)}
          onBlur={() => onNumBlur('w')}
/>
        <InputField
          label="H"
          type="number"
          value={local.h}
          min={1}
          max={canvas.h}
          onChange={(e) => onNumChange('h', e.target.value)}
          onBlur={() => onNumBlur('h')}
/>
      </div>

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
          <Button
            type="button"
            size="SM"
            theme="danger"
            text="Remove"
            onClick={onDelete}
          />
        )}
      </div>
    </div>
  );
}
