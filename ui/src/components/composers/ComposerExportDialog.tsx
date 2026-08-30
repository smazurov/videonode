import { useEffect, useState } from 'react';
import toast from 'react-hot-toast';
import { ClipboardDocumentIcon, DocumentArrowDownIcon } from '@heroicons/react/24/outline';

import { Button } from '../Button';
import { Spinner } from '../Spinner';
import { copyText } from '../../lib/clipboard';
import { useComposerStore } from '../../hooks/useComposerStore';

interface ComposerExportDialogProps {
  readonly composerId: string;
  readonly open: boolean;
  readonly onClose: () => void;
}

// Shows a composer's TOML for copy-paste, with copy-to-clipboard and download
// shortcuts. Fetches the document fresh each time it opens.
export function ComposerExportDialog({ composerId, open, onClose }: ComposerExportDialogProps) {
  const exportComposerToml = useComposerStore((s) => s.exportComposerToml);
  const [toml, setToml] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Reset transient state on the closed→open transition during render, so the
  // fetch effect below never has to call setState synchronously.
  const [prevOpen, setPrevOpen] = useState(open);
  if (prevOpen !== open) {
    setPrevOpen(open);
    if (!prevOpen && open) {
      setLoading(true);
      setError(null);
      setToml('');
    }
  }

  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    exportComposerToml(composerId)
      .then((text) => {
        if (!cancelled) setToml(text);
      })
      .catch((error_: unknown) => {
        if (!cancelled) {
          setError(error_ instanceof Error ? error_.message : 'Failed to export composer');
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [open, composerId, exportComposerToml]);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [open, onClose]);

  if (!open) return null;

  const handleCopy = async () => {
    try {
      await copyText(toml);
      toast.success('Copied TOML to clipboard');
    } catch {
      toast.error('Failed to copy to clipboard');
    }
  };

  const handleDownload = () => {
    const url = URL.createObjectURL(new Blob([toml], { type: 'application/toml' }));
    const link = document.createElement('a');
    link.href = url;
    link.download = `${composerId}.toml`;
    link.click();
    URL.revokeObjectURL(url);
  };

  let body;
  if (loading) {
    body = (
      <div className="flex h-72 items-center justify-center">
        <Spinner />
      </div>
    );
  } else if (error) {
    body = (
      <p className="text-sm text-danger-soft-fg" role="alert">
        {error}
      </p>
    );
  } else {
    body = (
      <textarea
        value={toml}
        readOnly
        spellCheck={false}
        className="h-72 w-full resize-y rounded-sm border border-border bg-surface-raised p-3 font-mono text-xs text-fg focus:outline-none"
      />
    );
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
      role="dialog"
      aria-modal="true"
      aria-labelledby="composer-export-title"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div className="w-full max-w-2xl rounded-md border border-border bg-surface p-6 shadow-lg space-y-4">
        <h2 id="composer-export-title" className="text-lg font-semibold text-fg">
          Export "{composerId}" as TOML
        </h2>

        {body}

        <div className="flex items-center justify-end gap-2">
          <Button theme="light" size="MD" text="Close" onClick={onClose} />
          <Button
            theme="light"
            size="MD"
            text="Download"
            LeadingIcon={DocumentArrowDownIcon}
            onClick={handleDownload}
            disabled={loading || !!error}
          />
          <Button
            theme="primary"
            size="MD"
            text="Copy"
            LeadingIcon={ClipboardDocumentIcon}
            onClick={handleCopy}
            disabled={loading || !!error}
          />
        </div>
      </div>
    </div>
  );
}
