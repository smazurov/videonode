import { useEffect, useRef, useState } from 'react';
import { DocumentArrowUpIcon } from '@heroicons/react/24/outline';

import { Button } from '../Button';

interface ComposerImportDialogProps {
  readonly open: boolean;
  readonly title: string;
  readonly description: string;
  readonly submitText: string;
  readonly onClose: () => void;
  readonly onImport: (toml: string) => Promise<void>;
}

// Paste-or-load dialog for importing a composer from TOML. Accepts pasted text
// directly or loads a .toml file into the same textarea, then hands the raw
// document to onImport. Errors surface inline; success closes the dialog.
export function ComposerImportDialog({
  open,
  title,
  description,
  submitText,
  onClose,
  onImport,
}: ComposerImportDialogProps) {
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [toml, setToml] = useState('');
  const [importing, setImporting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [prevOpen, setPrevOpen] = useState(open);
  if (prevOpen !== open) {
    setPrevOpen(open);
    if (!prevOpen && open) {
      setToml('');
      setError(null);
      setImporting(false);
    }
  }

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [open, onClose]);

  if (!open) return null;

  const handleSubmit = async () => {
    if (!toml.trim()) {
      setError('Paste or load a TOML document first.');
      return;
    }
    setImporting(true);
    setError(null);
    try {
      await onImport(toml);
      onClose();
    } catch (error_) {
      setError(error_ instanceof Error ? error_.message : 'Failed to import composer');
    } finally {
      setImporting(false);
    }
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
      role="dialog"
      aria-modal="true"
      aria-labelledby="composer-import-title"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div className="w-full max-w-2xl rounded-md border border-border bg-surface p-6 shadow-lg space-y-4">
        <h2 id="composer-import-title" className="text-lg font-semibold text-fg">
          {title}
        </h2>
        <p className="text-sm text-fg-subtle">{description}</p>

        <input
          ref={fileInputRef}
          type="file"
          accept=".toml,application/toml,text/plain"
          className="hidden"
          onChange={async (e) => {
            const file = e.currentTarget.files?.[0];
            e.currentTarget.value = '';
            if (file) setToml(await file.text());
          }}
        />

        <textarea
          value={toml}
          onChange={(e) => setToml(e.target.value)}
          spellCheck={false}
          placeholder="Paste composer TOML here…"
          className="h-72 w-full resize-y rounded-sm border border-border bg-surface-raised p-3 font-mono text-xs text-fg focus:outline-none focus:ring-1 focus:ring-accent"
        />

        {error && (
          <p className="text-sm text-danger-soft-fg" role="alert">
            {error}
          </p>
        )}

        <div className="flex items-center justify-between gap-2">
          <Button
            theme="light"
            size="MD"
            text="Load file…"
            LeadingIcon={DocumentArrowUpIcon}
            onClick={() => fileInputRef.current?.click()}
            disabled={importing}
          />
          <div className="flex items-center gap-2">
            <Button theme="light" size="MD" text="Cancel" onClick={onClose} disabled={importing} />
            <Button
              theme="primary"
              size="MD"
              text={submitText}
              onClick={handleSubmit}
              loading={importing}
              disabled={importing}
            />
          </div>
        </div>
      </div>
    </div>
  );
}
