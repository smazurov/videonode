import { useCallback, useState } from 'react';
import toast from 'react-hot-toast';
import { Button } from '../Button';
import { PerspectiveCanvas, type Corner, type SnapshotDims } from './PerspectiveCanvas';
import type { ComposerEffect } from '../../hooks/useComposerStore';

export type PerspectiveValue = [Corner, Corner, Corner, Corner];

interface PerspectiveEditorProps {
  composerId: string;
  inputRef: string;
  initialCorners: Corner[] | undefined;
  // Source id whose raw NV12 snapshot drives the live preview backdrop. Null
  // when the input ref does not resolve to a source (e.g. composer-as-input).
  snapshotSourceId: string | null;
  saving: boolean;
  onSave: (effect: ComposerEffect | null) => Promise<void>;
  onCancel: () => void;
}

export function PerspectiveEditor({
  inputRef,
  initialCorners,
  snapshotSourceId,
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
  const [snapshotDims, setSnapshotDims] = useState<SnapshotDims | null>(null);

  const handleCornersChange = useCallback((next: Corner[], isSorted: boolean) => {
    setCorners(next);
    setSorted(isSorted);
  }, []);

  const handleApply = useCallback(async () => {
    if (corners.length !== 4 || !snapshotDims) return;
    try {
      const corners4 = corners as PerspectiveValue;
      await onSave({
        type: 'perspective',
        corners: corners4,
        snapshot_w: snapshotDims.w,
        snapshot_h: snapshotDims.h,
      });
      toast.success(`Perspective applied to ${inputRef}`);
    } catch (error) {
      toast.error('Failed to apply perspective');
      console.error(error);
    }
  }, [corners, snapshotDims, inputRef, onSave]);

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
        snapshotSourceId={snapshotSourceId}
        corners={corners}
        sorted={sorted}
        onCornersChange={handleCornersChange}
        onSnapshotDimsChange={setSnapshotDims}
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
            disabled={saving || !snapshotDims}
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
