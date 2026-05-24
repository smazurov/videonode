import { useEffect, useRef, useState } from 'react';
import { Button } from '../Button';
import { useSourceStore, type SourceConsumerRef } from '../../hooks/useSourceStore';

interface SourceDeleteDialogProps {
  readonly sourceId: string;
  readonly consumers: SourceConsumerRef[];
  readonly open: boolean;
  readonly onClose: () => void;
  readonly onDeleted: () => void;
}

// Confirmation dialog for source deletion. Blocks the action whenever the
// source has consumers (composers or streams referencing it); lists them
// so the operator knows what to re-point or delete first.
export function SourceDeleteDialog({
  sourceId,
  consumers,
  open,
  onClose,
  onDeleted,
}: SourceDeleteDialogProps) {
  const dialogRef = useRef<HTMLDivElement | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const deleteSource = useSourceStore((s) => s.deleteSource);

  const blocked = consumers.length > 0;

  useEffect(() => {
    if (!open) {
      setError(null);
      setDeleting(false);
      return;
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [open, onClose]);

  if (!open) return null;

  const handleConfirm = async () => {
    if (blocked) return;
    setDeleting(true);
    setError(null);
    try {
      await deleteSource(sourceId);
      onDeleted();
    } catch (error_) {
      setError(error_ instanceof Error ? error_.message : 'Failed to delete source');
    } finally {
      setDeleting(false);
    }
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
      role="dialog"
      aria-modal="true"
      aria-labelledby="source-delete-title"
      ref={dialogRef}
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div className="w-full max-w-md rounded-md border border-border bg-surface p-6 shadow-lg space-y-4">
        <h2 id="source-delete-title" className="text-lg font-semibold text-fg">
          Delete source “{sourceId}”?
        </h2>

        {blocked ? (
          <div className="space-y-2">
            <p className="text-sm text-fg">
              This source cannot be deleted because it is referenced by{' '}
              {consumers.length} consumer
              {consumers.length === 1 ? '' : 's'}:
            </p>
            <ul className="rounded-sm border border-border divide-y divide-border max-h-40 overflow-y-auto">
              {consumers.map((c) => (
                <li
                  key={`${c.kind}:${c.id}`}
                  className="px-3 py-2 text-sm font-mono text-fg flex items-center justify-between"
                >
                  <span>{c.id}</span>
                  <span className="text-xs text-fg-subtle">{c.kind}</span>
                </li>
              ))}
            </ul>
            <p className="text-xs text-fg-subtle">
              Re-point or delete these consumers first, then try again.
            </p>
          </div>
        ) : (
          <p className="text-sm text-fg-subtle">
            This will permanently remove the source and stop its capture
            process. This cannot be undone.
          </p>
        )}

        {error && (
          <p className="text-sm text-danger-soft-fg" role="alert">
            {error}
          </p>
        )}

        <div className="flex items-center justify-end gap-2">
          <Button
            theme="light"
            size="MD"
            onClick={onClose}
            text="Cancel"
            disabled={deleting}
          />
          <Button
            theme="danger"
            size="MD"
            onClick={handleConfirm}
            text="Delete"
            disabled={blocked || deleting}
            loading={deleting}
          />
        </div>
      </div>
    </div>
  );
}
