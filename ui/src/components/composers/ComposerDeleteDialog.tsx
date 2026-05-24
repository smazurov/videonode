import { useEffect, useMemo, useRef, useState } from 'react';
import { Button } from '../Button';
import { API_BASE_URL } from '../../lib/api';
import { getAuthCredentials } from '../../lib/auth';

// Minimal shape needed to flag a referencing stream.
export interface StreamLikeRef {
  stream_id: string;
  upstream?: string | null;
}

interface ComposerDeleteDialogProps {
  readonly composerId: string;
  readonly streams: StreamLikeRef[];
  readonly open: boolean;
  readonly onClose: () => void;
  readonly onDeleted: () => void;
}

// Confirmation dialog for composer deletion. Blocks the action whenever any
// stream's upstream points at `composer:<composerId>`; lists the offending
// streams so the operator knows what to re-point first.
export function ComposerDeleteDialog({
  composerId,
  streams,
  open,
  onClose,
  onDeleted,
}: ComposerDeleteDialogProps) {
  const dialogRef = useRef<HTMLDivElement | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const referencingStreams = useMemo(() => {
    const target = `composer:${composerId}`;
    return streams.filter((s) => s.upstream === target);
  }, [streams, composerId]);

  const blocked = referencingStreams.length > 0;

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
      const credentials = getAuthCredentials();
      const headers: Record<string, string> = {};
      if (credentials) headers.Authorization = `Basic ${credentials}`;
      const response = await fetch(
        `${API_BASE_URL}/api/composers/${encodeURIComponent(composerId)}`,
        { method: 'DELETE', headers },
      );
      if (!response.ok) {
        const text = await response.text().catch(() => '');
        throw new Error(text || `Failed to delete composer (${response.status})`);
      }
      onDeleted();
    } catch (error_) {
      setError(error_ instanceof Error ? error_.message : 'Failed to delete composer');
    } finally {
      setDeleting(false);
    }
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
      role="dialog"
      aria-modal="true"
      aria-labelledby="composer-delete-title"
      ref={dialogRef}
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div className="w-full max-w-md rounded-md border border-border bg-surface p-6 shadow-lg space-y-4">
        <h2
          id="composer-delete-title"
          className="text-lg font-semibold text-fg"
        >
          Delete composer “{composerId}”?
        </h2>

        {blocked ? (
          <div className="space-y-2">
            <p className="text-sm text-fg">
              This composer cannot be deleted because it is referenced by{' '}
              {referencingStreams.length} stream
              {referencingStreams.length === 1 ? '' : 's'}:
            </p>
            <ul className="rounded-sm border border-border divide-y divide-border max-h-40 overflow-y-auto">
              {referencingStreams.map((s) => (
                <li
                  key={s.stream_id}
                  className="px-3 py-2 text-sm font-mono text-fg"
                >
                  {s.stream_id}
                </li>
              ))}
            </ul>
            <p className="text-xs text-fg-subtle">
              Re-point or delete these streams first, then try again.
            </p>
          </div>
        ) : (
          <p className="text-sm text-fg-subtle">
            This will permanently remove the composer and its layout. Sources
            referenced by this composer are kept.
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
