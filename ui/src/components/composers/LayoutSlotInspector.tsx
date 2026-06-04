import { useState } from "react";
import {
  ArrowUturnLeftIcon,
  ArrowUturnRightIcon,
  TrashIcon,
} from "@heroicons/react/24/outline";

import { Button } from "../Button";
import { InputField } from "../InputField";
import { Select } from "../Select";
import type {
  CanvasDims,
  ComposerInput,
  LayoutSlot,
} from "../../lib/composer-types";
import {
  type ContentTransform,
  type FillMode,
  applyContentToSlot,
  backendToContent,
} from "./region-content";

type PctKey = "panX" | "panY" | "zoom";

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

// Right-pane editor for the selected slot. A Region section controls the
// on-canvas rectangle (x/y/w/h); a Content section controls the source overlay
// inside it — rotation, fill mode, and (in cover) pan/zoom. Rotation is a
// Content property: the Region box never rotates.
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
  // Track in-progress edits only while an input is focused; otherwise values
  // come straight from the slot prop (zero-delay sync).
  const [editing, setEditing] = useState<Record<string, string>>({});

  if (!slot) {
    return (
      <div className="rounded-md border border-border bg-surface p-4 text-sm text-fg-subtle">
        Select a region to edit it.
      </div>
    );
  }

  const content = backendToContent(slot);

  type NumKey = "x" | "y" | "w" | "h";
  const numVal = (key: NumKey) =>
    key in editing ? editing[key]! : String(slot[key]);
  const clampNum = (key: NumKey, v: number) => {
    if (key === "w") return Math.max(1, Math.min(canvas.w, v));
    if (key === "h") return Math.max(1, Math.min(canvas.h, v));
    if (key === "x") return Math.max(0, Math.min(canvas.w - slot.w, v));
    return Math.max(0, Math.min(canvas.h - slot.h, v));
  };
  const onNumFocus = (key: string) =>
    setEditing((e) => ({ ...e, [key]: String(slot[key as NumKey]) }));
  const onNumChange = (key: NumKey, raw: string) => {
    setEditing((e) => ({ ...e, [key]: raw }));
    const parsed = Number.parseInt(raw, 10);
    if (!Number.isNaN(parsed))
      onChange({ ...slot, [key]: clampNum(key, parsed) });
  };
  const onNumBlur = (key: string) =>
    setEditing((e) => {
      const r = { ...e };
      delete r[key];
      return r;
    });

  // Apply a content change: map the patched transform back to wire fields,
  // clearing a stale crop when the fill mode isn't cover.
  const setContent = (patch: Partial<ContentTransform>) =>
    onChange(applyContentToSlot(slot, { ...content, ...patch }));

  const pctVal = (key: PctKey) =>
    key in editing ? editing[key]! : String(Math.round(content[key] * 100));
  // Seed the edit buffer from the content transform — NOT from slot[key], which
  // has no panX/panY/zoom and would show "undefined".
  const onPctFocus = (key: PctKey) =>
    setEditing((e) => ({
      ...e,
      [key]: String(Math.round(content[key] * 100)),
    }));
  const onPctChange = (key: PctKey, raw: string) => {
    setEditing((e) => ({ ...e, [key]: raw }));
    const v = Number.parseInt(raw, 10);
    if (Number.isNaN(v)) return;
    if (key === "zoom") setContent({ zoom: Math.max(1, v / 100) });
    else setContent({ [key]: Math.max(0, Math.min(100, v)) / 100 });
  };

  const rotateBy = (delta: number) =>
    setContent({
      rotation: ((((content.rotation + delta) % 360) + 360) %
        360) as ContentTransform["rotation"],
    });

  return (
    <div className="rounded-md border border-border bg-surface p-4 space-y-4">
      <Select
        label="Source"
        value={slot.input}
        onChange={(e) => onChangeInputRef(e.target.value)}
      >
        {inputs.map((input) => (
          <option key={input.ref} value={input.ref}>
            {input.ref}
            {input.effect ? ` · ${input.effect.type}` : ""}
          </option>
        ))}
      </Select>

      {/* Region: the on-canvas rectangle. */}
      <div className="space-y-2">
        <h3 className="text-sm font-semibold text-fg">Region</h3>
        <div className="grid grid-cols-2 gap-3">
          {(["x", "y", "w", "h"] as const).map((key) => (
            <InputField
              key={key}
              label={key.toUpperCase()}
              type="number"
              value={numVal(key)}
              min={key === "w" || key === "h" ? 1 : 0}
              max={key === "x" || key === "w" ? canvas.w : canvas.h}
              onFocus={() => onNumFocus(key)}
              onChange={(e) => onNumChange(key, e.target.value)}
              onBlur={() => onNumBlur(key)}
            />
          ))}
        </div>
      </div>

      {/* Content: the source overlay inside the region. */}
      <div className="space-y-2 border-t border-border pt-3">
        <h3 className="text-sm font-semibold text-fg">Content</h3>

        <div className="space-y-1">
          <label className="block text-sm font-medium text-fg">Rotation</label>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => rotateBy(-90)}
              title="Rotate content 90° counter-clockwise"
              aria-label="Rotate content 90° counter-clockwise"
              className="rounded-sm p-1.5 text-fg-muted hover:bg-surface-muted"
            >
              <ArrowUturnLeftIcon className="h-4 w-4" />
            </button>
            <div className="flex-1">
              <Select
                value={String(content.rotation)}
                onChange={(e) =>
                  setContent({
                    rotation: Number.parseInt(
                      e.target.value,
                      10,
                    ) as ContentTransform["rotation"],
                  })
                }
              >
                <option value="0">0°</option>
                <option value="90">90°</option>
                <option value="180">180°</option>
                <option value="270">270°</option>
              </Select>
            </div>
            <button
              type="button"
              onClick={() => rotateBy(90)}
              title="Rotate content 90° clockwise"
              aria-label="Rotate content 90° clockwise"
              className="rounded-sm p-1.5 text-fg-muted hover:bg-surface-muted"
            >
              <ArrowUturnRightIcon className="h-4 w-4" />
            </button>
          </div>
        </div>

        <Select
          label="Fill"
          value={content.fill}
          onChange={(e) => setContent({ fill: e.target.value as FillMode })}
        >
          <option value="cover">Cover (fill)</option>
          <option value="fit">Fit (letterbox)</option>
          <option value="stretch">Stretch</option>
        </Select>

        {content.fill === "cover" && (
          <div className="grid grid-cols-3 gap-3">
            <InputField
              label="Pan X %"
              type="number"
              value={pctVal("panX")}
              min={0}
              max={100}
              onFocus={() => onPctFocus("panX")}
              onChange={(e) => onPctChange("panX", e.target.value)}
              onBlur={() => onNumBlur("panX")}
            />
            <InputField
              label="Pan Y %"
              type="number"
              value={pctVal("panY")}
              min={0}
              max={100}
              onFocus={() => onPctFocus("panY")}
              onChange={(e) => onPctChange("panY", e.target.value)}
              onBlur={() => onNumBlur("panY")}
            />
            <InputField
              label="Zoom %"
              type="number"
              value={pctVal("zoom")}
              min={100}
              onFocus={() => onPctFocus("zoom")}
              onChange={(e) => onPctChange("zoom", e.target.value)}
              onBlur={() => onNumBlur("zoom")}
            />
          </div>
        )}
      </div>

      <div className="flex flex-wrap gap-2 border-t border-border pt-3">
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
            title="Remove region"
            className="rounded-sm p-1.5 text-danger hover:bg-danger/10"
          >
            <TrashIcon className="h-4 w-4" />
          </button>
        )}
      </div>
    </div>
  );
}
