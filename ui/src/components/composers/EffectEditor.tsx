import { useMemo, useState } from 'react';
import { Select } from '../Select';
import { PerspectiveEditor } from './PerspectiveEditor';
import type { ComposerEffect } from '../../hooks/useComposerStore';
import type { Corner } from './PerspectiveCanvas';

// Effect kinds the editor knows about. v1 supports perspective only; future
// types appear in the dropdown disabled so the schema-extension story is
// visible in the UI without dragging in unfinished plumbing.
type EffectType = 'perspective' | 'color' | 'crop';

const EFFECT_OPTIONS: ReadonlyArray<{ value: EffectType; label: string; enabled: boolean }> = [
  { value: 'perspective', label: 'Perspective', enabled: true },
  { value: 'color', label: 'Color (coming soon)', enabled: false },
  { value: 'crop', label: 'Crop (coming soon)', enabled: false },
];

interface EffectEditorProps {
  composerId: string;
  inputRef: string;
  effect: ComposerEffect | null | undefined;
  snapshotStreamId: string | null;
  inputWidth: number;
  inputHeight: number;
  saving: boolean;
  onSave: (effect: ComposerEffect | null) => Promise<void>;
  onCancel: () => void;
}

export function EffectEditor({
  composerId,
  inputRef,
  effect,
  snapshotStreamId,
  inputWidth,
  inputHeight,
  saving,
  onSave,
  onCancel,
}: Readonly<EffectEditorProps>) {
  const initialType: EffectType = (effect?.type as EffectType | undefined) ?? 'perspective';
  const [type, setType] = useState<EffectType>(initialType);

  const initialCorners: Corner[] | undefined = useMemo(() => {
    if (!effect || effect.type !== 'perspective' || !effect.corners) return undefined;
    return effect.corners
      .filter((c): c is number[] => c != null && c.length >= 2)
      .map((c) => [c[0], c[1]] as Corner);
  }, [effect]);

  return (
    <div className="space-y-4 rounded-md border border-border bg-surface p-4">
      <div className="flex items-center justify-between gap-3">
        <h4 className="text-sm font-medium text-fg">
          Effect — <span className="font-mono text-fg-muted">{inputRef}</span>
        </h4>
        <div className="w-56">
          <Select
            label="Type"
            value={type}
            onChange={(e) => setType(e.target.value as EffectType)}
            disabled={saving}
          >
            {EFFECT_OPTIONS.map((opt) => (
              <option key={opt.value} value={opt.value} disabled={!opt.enabled}>
                {opt.label}
              </option>
            ))}
          </Select>
        </div>
      </div>

      {type === 'perspective' && (
        <PerspectiveEditor
          composerId={composerId}
          inputRef={inputRef}
          initialCorners={initialCorners}
          snapshotStreamId={snapshotStreamId}
          inputWidth={inputWidth}
          inputHeight={inputHeight}
          saving={saving}
          onSave={onSave}
          onCancel={onCancel}
        />
      )}

      {type !== 'perspective' && (
        <p className="text-sm text-fg-subtle">
          This effect type is not implemented yet. Pick <strong>Perspective</strong> to edit.
        </p>
      )}
    </div>
  );
}
