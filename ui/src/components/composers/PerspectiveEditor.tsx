import { useCallback, useState } from 'react';
import toast from 'react-hot-toast';
import { Button } from '../Button';
import { PerspectiveCanvas, type Corner } from './PerspectiveCanvas';
import type { ComposerEffect } from '../../hooks/useComposerStore';

export type PerspectiveValue = [Corner, Corner, Corner, Corner];

interface PerspectiveEditorProps {
  composerId: string;
  inputRef: string;
  initialCorners: Corner[] | undefined;
  // Snapshot source: a stream id whose snapshot serves as the live preview.
  // The dedicated per-source snapshot endpoint will swap in once it ships.
  snapshotStreamId: string | null;
  inputWidth: number;
  inputHeight: number;
  saving: boolean;
  onSave: (effect: ComposerEffect | null) => Promise<void>;
  onCancel: () => void;
}

export function PerspectiveEditor({
  inputRef,
  initialCorners,
  snapshotStreamId,
  inputWidth,
  inputHeight,
  saving,
  onSave,
  onCancel,
}: Readonly<PerspectiveEditorProps>) {
  // Seed local state from props on mount and whenever the parent swaps the
  // edited input (identity change on initialCorners). Tracking the seed in
  // useState (vs ref/effect) lets render-phase reset stay lint-clean.
  const initialSeed = (): Corner[] =>
    initialCorners && initialCorners.length === 4 ? [...initialCorners] : [];
  const [corners, setCorners] = useState<Corner[]>(initialSeed);
  const [sorted, setSorted] = useState(() => initialCorners?.length === 4);
  const [lastSeed, setLastSeed] = useState(initialCorners);
  if (lastSeed !== initialCorners) {
    setLastSeed(initialCorners);
    setCorners(initialSeed());
    setSorted(initialCorners?.length === 4);
  }

  const handleCornersChange = useCallback((next: Corner[], isSorted: boolean) => {
    setCorners(next);
    setSorted(isSorted);
  }, []);

  const handleApply = useCallback(async () => {
    if (corners.length !== 4) return;
    try {
      const corners4 = corners as PerspectiveValue;
      await onSave({ type: 'perspective', corners: corners4 });
      toast.success(`Perspective applied to ${inputRef}`);
    } catch (error) {
      toast.error('Failed to apply perspective');
      console.error(error);
    }
  }, [corners, inputRef, onSave]);

  const handleClear = useCallback(() => {
    setCorners([]);
    setSorted(false);
  }, []);

  const handleRemove = useCallback(async () => {
    try {
      await onSave(null);
      toast.success(`Perspective cleared from ${inputRef}`);
    } catch (error) {
      toast.error('Failed to clear perspective');
      console.error(error);
    }
  }, [inputRef, onSave]);

  const hadInitial = initialCorners?.length === 4;

  return (
    <div className="space-y-3">
      <p className="text-sm text-fg-subtle">
        {corners.length < 4
          ? `Click 4 points on the preview to define the region. (${corners.length}/4)`
          : 'Drag corners to adjust. Click Apply to save.'}
      </p>
      <PerspectiveCanvas
        snapshotStreamId={snapshotStreamId}
        corners={corners}
        sorted={sorted}
        onCornersChange={handleCornersChange}
        inputWidth={inputWidth}
        inputHeight={inputHeight}
      />
      <div className="flex items-center gap-2">
        {corners.length > 0 && (
          <Button theme="light" size="SM" text="Clear" onClick={handleClear} disabled={saving} />
        )}
        {corners.length === 4 && (
          <Button
            theme="primary"
            size="SM"
            text={saving ? 'Applying...' : 'Apply'}
            onClick={handleApply}
            disabled={saving}
          />
        )}
        {corners.length === 0 && hadInitial && (
          <Button
            theme="danger"
            size="SM"
            text={saving ? 'Removing...' : 'Remove Perspective'}
            onClick={handleRemove}
            disabled={saving}
          />
        )}
        <Button theme="light" size="SM" text="Done" onClick={onCancel} disabled={saving} />
      </div>
    </div>
  );
}
