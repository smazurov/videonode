import { useEffect, useState } from 'react';
import toast from 'react-hot-toast';
import { ClipboardDocumentIcon, DocumentArrowDownIcon } from '@heroicons/react/24/outline';

import { Button } from '../Button';
import { encodeCanvasSize, encodeClip } from './canvas-mask';
import { renderMaskPNG } from './mask-png';
import type { ComposerMask } from '../../hooks/useComposerMask';
import { API_BASE_URL } from '../../lib/api_fetch';
import { copyText } from '../../lib/clipboard';

interface MaskExportDialogProps {
  readonly mask: ComposerMask;
  /** Stream to play in the browser-source URL. Omitted when no stream reads the composer. */
  readonly streamId?: string | undefined;
  readonly open: boolean;
  readonly onClose: () => void;
}

const NO_STREAM_HINT = 'needs a stream that reads from this composer; none exists yet.';

const PREVIEW_W = 480;

/**
 * Exports a composer's video coverage as an OBS alpha mask: a canvas-sized PNG
 * for the Image Mask/Blend filter, or a browser-source URL that clips the live
 * player to the same regions.
 */
export function MaskExportDialog({ mask, streamId, open, onClose }: MaskExportDialogProps) {
  const { composerId, canvas, rects, unsizedInputs } = mask;
  const [downloading, setDownloading] = useState(false);

  const browserSourceUrl = streamId
    ? `${API_BASE_URL}/video?stream=${encodeURIComponent(streamId)}` +
      `&canvas=${encodeCanvasSize(canvas)}&clip=${encodeClip(rects)}`
    : null;

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [open, onClose]);

  if (!open) return null;

  const handleDownload = async () => {
    setDownloading(true);
    try {
      const blob = await renderMaskPNG(rects, canvas);
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = `${composerId}-mask.png`;
      link.click();
      URL.revokeObjectURL(url);
    } catch {
      toast.error('Failed to render mask PNG');
    } finally {
      setDownloading(false);
    }
  };

  const handleCopyUrl = async () => {
    if (!browserSourceUrl) return;
    try {
      await copyText(browserSourceUrl);
      toast.success('Copied browser source URL');
    } catch {
      toast.error('Failed to copy to clipboard');
    }
  };

  const previewH = Math.round((PREVIEW_W * canvas.h) / Math.max(1, canvas.w));

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
      role="dialog"
      aria-modal="true"
      aria-labelledby="mask-export-title"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div className="w-full max-w-2xl rounded-md border border-border bg-surface p-6 shadow-lg space-y-4">
        <h2 id="mask-export-title" className="text-lg font-semibold text-fg">
          OBS alpha mask for "{composerId}"
        </h2>

        <p className="text-sm text-fg-subtle">
          Opaque where the compositor paints video, transparent everywhere else —
          canvas background and letterbox bars are masked off.
        </p>

        <div className="flex justify-center">
          <svg
            width={PREVIEW_W}
            height={previewH}
            viewBox={`0 0 ${canvas.w} ${canvas.h}`}
            className="rounded-sm border border-border bg-surface-sunken text-fg"
            role="img"
            aria-label={`Mask preview: ${rects.length} video regions on a ${canvas.w}x${canvas.h} canvas`}
          >
            {rects.map((r) => (
              <rect
                key={`${r.x},${r.y},${r.w},${r.h}`}
                x={r.x}
                y={r.y}
                width={r.w}
                height={r.h}
                fill="currentColor"
              />
            ))}
          </svg>
        </div>

        {rects.length === 0 && (
          <p className="text-sm text-danger-soft-fg" role="alert">
            This composer has no on-canvas video regions, so the mask would be fully
            transparent.
          </p>
        )}

        {unsizedInputs.length > 0 && (
          <p className="text-sm text-warning-soft-fg" role="alert">
            No frame size reported for {unsizedInputs.join(', ')} — their letterbox bars
            are unknown and stay unmasked. Start the pipeline and reopen this dialog.
          </p>
        )}

        <div className="space-y-2 text-sm text-fg-subtle">
          <p>
            <span className="font-medium text-fg">Media source:</span> download the PNG,
            then add an Image Mask/Blend filter to your SRT or RTSP source and pick
            "Alpha Mask (Alpha Channel)".
          </p>
          <p>
            <span className="font-medium text-fg">Browser source:</span>{' '}
            {browserSourceUrl ? (
              <>
                use the URL below at {canvas.w}x{canvas.h}, with "Shutdown source when
                not visible" on. Re-copy it after editing the layout.
              </>
            ) : (
              NO_STREAM_HINT
            )}
          </p>
          {browserSourceUrl && (
            <code className="block truncate rounded-sm border border-border bg-surface-raised p-2 font-mono text-xs text-fg">
              {browserSourceUrl}
            </code>
          )}
        </div>

        <div className="flex items-center justify-end gap-2">
          <Button theme="light" size="MD" text="Close" onClick={onClose} />
          {browserSourceUrl && (
            <Button
              theme="light"
              size="MD"
              text="Copy URL"
              LeadingIcon={ClipboardDocumentIcon}
              onClick={handleCopyUrl}
            />
          )}
          <Button
            theme="primary"
            size="MD"
            text="Download PNG"
            LeadingIcon={DocumentArrowDownIcon}
            onClick={handleDownload}
            disabled={downloading || rects.length === 0}
          />
        </div>
      </div>
    </div>
  );
}
