import { useMemo, useState } from 'react';
import toast from 'react-hot-toast';

import { Button } from '../Button';
import { Card } from '../Card';
import { InputField } from '../InputField';
import type { ComposerData } from '../../lib/composer-types';
import {
  DEFAULT_CANVAS_BACKGROUND,
  DEFAULT_CANVAS_FPS,
  hasCanvasErrors,
  validateBackground,
  validateCanvas,
} from '../../lib/composer-types';
import { useComposerStore } from '../../hooks/useComposerStore';

interface ComposerCanvasEditorProps {
  composer: ComposerData;
}

interface CanvasDraft {
  w: number;
  h: number;
  fps: number;
  bg: string;
  // The committed prop values this draft was seeded from, used to detect
  // external changes (SSE, another tab) and reset without a sync-setState effect.
  seededW: number;
  seededH: number;
  seededFps: number;
  seededBg: string;
}

// Normalize a hex background to a comparable canonical form: lowercase,
// leading '#'. An unset/empty background is the composer's opaque-black
// default, so it normalizes to DEFAULT_CANVAS_BACKGROUND.
function normalizeBg(background: string | undefined): string {
  if (!background) return DEFAULT_CANVAS_BACKGROUND;
  const lower = background.toLowerCase();
  return lower.startsWith('#') ? lower : `#${lower}`;
}

interface CanvasCommitted {
  w: number;
  h: number;
  fps: number;
  bg: string;
}

// Reconcile a draft against the committed (persisted) values. Returns the
// re-seeded draft when the persisted values changed externally (SSE,
// another tab), preserving any field the user has actively edited; returns
// null when the draft is already in sync. Keeping this pure pulls the
// branchy reset logic out of the component render body.
function reconcileDraft(draft: CanvasDraft, p: CanvasCommitted): CanvasDraft | null {
  if (
    draft.seededW === p.w &&
    draft.seededH === p.h &&
    draft.seededFps === p.fps &&
    draft.seededBg === p.bg
  ) {
    return null;
  }
  return {
    w: draft.w === draft.seededW ? p.w : draft.w,
    h: draft.h === draft.seededH ? p.h : draft.h,
    fps: draft.fps === draft.seededFps ? p.fps : draft.fps,
    bg: draft.bg === draft.seededBg ? p.bg : draft.bg,
    seededW: p.w,
    seededH: p.h,
    seededFps: p.fps,
    seededBg: p.bg,
  };
}

// Canvas dim + fps editor with explicit Apply. Changing any of these
// fields restarts the videonode-composer process (and drops downstream
// encoders momentarily), so we don't auto-save on every keystroke the
// way the layout editor does.
export function ComposerCanvasEditor({ composer }: Readonly<ComposerCanvasEditorProps>) {
  const updateComposer = useComposerStore((s) => s.updateComposer);
  const persistedFps = composer.canvas.fps ?? DEFAULT_CANVAS_FPS;
  const persistedBg = normalizeBg(composer.canvas.background);

  const [draft, setDraft] = useState<CanvasDraft>(() => ({
    w: composer.canvas.w,
    h: composer.canvas.h,
    fps: persistedFps,
    bg: persistedBg,
    seededW: composer.canvas.w,
    seededH: composer.canvas.h,
    seededFps: persistedFps,
    seededBg: persistedBg,
  }));
  const [saving, setSaving] = useState(false);

  // When props change externally (SSE, another tab), derive the reset during
  // render by updating state directly — React re-renders immediately without
  // painting the intermediate state, avoiding the cascading-render problem of
  // synchronous setState inside a useEffect body.
  const reconciled = reconcileDraft(draft, {
    w: composer.canvas.w,
    h: composer.canvas.h,
    fps: persistedFps,
    bg: persistedBg,
  });
  if (reconciled) setDraft(reconciled);
  const { w, h, fps, bg } = reconciled ?? draft;

  const errors = useMemo(() => validateCanvas({ w, h, fps }), [w, h, fps]);
  const bgError = useMemo(() => validateBackground(bg), [bg]);
  const normalizedBg = normalizeBg(bg);
  const dimsDirty = w !== composer.canvas.w || h !== composer.canvas.h || fps !== persistedFps;
  const bgDirty = !bgError && normalizedBg !== persistedBg;
  const dirty = dimsDirty || bgDirty;
  const canApply = dirty && !hasCanvasErrors(errors) && !bgError && !saving;

  const handleApply = async () => {
    setSaving(true);
    try {
      // Send fps only when it diverges from the daemon default, and the
      // background only when non-default — keeps the persisted TOML clean
      // for the common case. Background is echoed on any canvas patch (the
      // PATCH replaces the whole canvas struct) so a dims edit never wipes a
      // custom color.
      const canvas: { w: number; h: number; fps?: number; background?: string } = { w, h };
      if (fps !== DEFAULT_CANVAS_FPS) canvas.fps = fps;
      if (normalizedBg !== DEFAULT_CANVAS_BACKGROUND) canvas.background = normalizedBg;
      await updateComposer(composer.composer_id, { canvas });
      toast.success('Canvas updated');
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Failed to update canvas');
    } finally {
      setSaving(false);
    }
  };

  const handleRevert = () => {
    setDraft({
      w: composer.canvas.w,
      h: composer.canvas.h,
      fps: persistedFps,
      bg: persistedBg,
      seededW: composer.canvas.w,
      seededH: composer.canvas.h,
      seededFps: persistedFps,
      seededBg: persistedBg,
    });
  };

  return (
    <Card>
      <Card.Header>
        <h2 className="text-sm font-semibold text-fg">Canvas</h2>
      </Card.Header>
      <Card.Content>
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <InputField
            label="Width (px)"
            type="number"
            min={16}
            max={7680}
            step={2}
            value={w}
            onChange={(e) => setDraft((d) => ({ ...d, w: parseInt(e.target.value, 10) || 0 }))}
            {...(errors.w ? { error: errors.w } : {})}
            fullWidth
          />
          <InputField
            label="Height (px)"
            type="number"
            min={16}
            max={4320}
            step={2}
            value={h}
            onChange={(e) => setDraft((d) => ({ ...d, h: parseInt(e.target.value, 10) || 0 }))}
            {...(errors.h ? { error: errors.h } : {})}
            fullWidth
          />
          <InputField
            label="Frame rate (fps)"
            type="number"
            min={1}
            max={240}
            step={1}
            value={fps}
            onChange={(e) => setDraft((d) => ({ ...d, fps: parseInt(e.target.value, 10) || 0 }))}
            {...(errors.fps ? { error: errors.fps } : {})}
            fullWidth
          />
          <div className="space-y-1">
            <span className="block text-sm font-medium text-fg">Background</span>
            <div className="flex items-center gap-2">
              <input
                type="color"
                aria-label="Background color"
                className="h-9 w-10 shrink-0 cursor-pointer rounded-md border border-border bg-transparent p-1"
                value={normalizedBg.slice(0, 7)}
                onChange={(e) => setDraft((d) => ({ ...d, bg: e.target.value }))}
              />
              <InputField
                label=""
                type="text"
                placeholder={DEFAULT_CANVAS_BACKGROUND}
                value={bg}
                onChange={(e) => setDraft((d) => ({ ...d, bg: e.target.value }))}
                {...(bgError ? { error: bgError } : {})}
                fullWidth
              />
            </div>
          </div>
        </div>
        <p className="mt-2 text-xs text-fg-subtle">
          Background: sources composite on top of this color. #RRGGBB or #RRGGBBAA; applies live.
        </p>
        <div className="mt-4 flex items-center justify-between gap-3">
          {dimsDirty ? (
            <p className="rounded-md border border-warning/40 bg-warning/10 px-2 py-1 text-xs text-warning-soft-fg">
              Applying restarts the composer process; downstream encoders briefly reconnect.
            </p>
          ) : (
            <p className="text-xs text-fg-subtle">
              Current: {composer.canvas.w}×{composer.canvas.h} @ {persistedFps} fps
            </p>
          )}
          <div className="flex gap-2">
            <Button
              theme="light"
              size="SM"
              text="Revert"
              onClick={handleRevert}
              disabled={!dirty || saving}
            />
            <Button
              theme="primary"
              size="SM"
              text={saving ? 'Applying…' : 'Apply'}
              onClick={handleApply}
              disabled={!canApply}
            />
          </div>
        </div>
      </Card.Content>
    </Card>
  );
}
