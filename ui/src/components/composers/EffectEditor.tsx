import { useMemo, useState } from 'react';
import { Select } from '../Select';
import { AutoCropEditor } from './AutoCropEditor';
import { PerspectiveEditor } from './PerspectiveEditor';
import type { ComposerEffect } from '../../hooks/useComposerStore';
import type { Corner } from './PerspectiveCanvas';

// Effect kinds the editor knows about. Perspective is a composer-native GPU
// effect; auto_crop is a daemon-level effect (AI tap drives the input's crop).
// Future types appear disabled so the schema-extension story is visible in the
// UI without dragging in unfinished plumbing.
type EffectType = 'perspective' | 'auto_crop' | 'color';

const EFFECT_OPTIONS: ReadonlyArray<{ value: EffectType; label: string; enabled: boolean }> = [
  { value: 'perspective', label: 'Perspective', enabled: true },
  { value: 'auto_crop', label: 'Auto-crop (AI)', enabled: true },
  { value: 'color', label: 'Color (coming soon)', enabled: false },
];

interface EffectEditorProps {
  composerId: string;
  inputRef: string;
  effect: ComposerEffect | null | undefined;
  snapshotSourceId: string | null;
  saving: boolean;
  onSave: (effect: ComposerEffect | null) => Promise<void>;
  onCancel: () => void;
}

export function EffectEditor({
  composerId,
  inputRef,
  effect,
  snapshotSourceId,
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
          snapshotSourceId={snapshotSourceId}
          saving={saving}
          onSave={onSave}
          onCancel={onCancel}
        />
      )}

      {type === 'auto_crop' && (
        <AutoCropEditor
          inputRef={inputRef}
          effect={effect}
          saving={saving}
          onSave={onSave}
          onCancel={onCancel}
        />
      )}

      {type !== 'perspective' && type !== 'auto_crop' && (
        <p className="text-sm text-fg-subtle">
          This effect type is not implemented yet. Pick <strong>Perspective</strong> or{' '}
          <strong>Auto-crop</strong> to edit.
        </p>
      )}
    </div>
  );
}
