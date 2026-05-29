import { useMemo, useState } from 'react';
import toast from 'react-hot-toast';

import { Button } from '../Button';
import { Card } from '../Card';
import { InputField } from '../InputField';
import type { ComposerData } from '../../lib/composer-types';
import {
  DEFAULT_CANVAS_FPS,
  hasCanvasErrors,
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
  // The committed prop values this draft was seeded from, used to detect
  // external changes (SSE, another tab) and reset without a sync-setState effect.
  seededW: number;
  seededH: number;
  seededFps: number;
}

// Canvas dim + fps editor with explicit Apply. Changing any of these
// fields restarts the videonode-composer process (and drops downstream
// encoders momentarily), so we don't auto-save on every keystroke the
// way the layout editor does.
export function ComposerCanvasEditor({ composer }: Readonly<ComposerCanvasEditorProps>) {
  const updateComposer = useComposerStore((s) => s.updateComposer);
  const persistedFps = composer.canvas.fps ?? DEFAULT_CANVAS_FPS;

  const [draft, setDraft] = useState<CanvasDraft>(() => ({
    w: composer.canvas.w,
    h: composer.canvas.h,
    fps: persistedFps,
    seededW: composer.canvas.w,
    seededH: composer.canvas.h,
    seededFps: persistedFps,
  }));
  const [saving, setSaving] = useState(false);

  // When props change externally (SSE, another tab), derive the reset during
  // render by updating state directly — React re-renders immediately without
  // painting the intermediate state, avoiding the cascading-render problem of
  // synchronous setState inside a useEffect body.
  const { w: draftW, h: draftH, fps: draftFps, seededW, seededH, seededFps } = draft;
  let w = draftW;
  let h = draftH;
  let fps = draftFps;
  if (
    seededW !== composer.canvas.w ||
    seededH !== composer.canvas.h ||
    seededFps !== persistedFps
  ) {
    const nextW = draftW === seededW ? composer.canvas.w : draftW;
    const nextH = draftH === seededH ? composer.canvas.h : draftH;
    const nextFps = draftFps === seededFps ? persistedFps : draftFps;
    w = nextW;
    h = nextH;
    fps = nextFps;
    setDraft({
      w: nextW,
      h: nextH,
      fps: nextFps,
      seededW: composer.canvas.w,
      seededH: composer.canvas.h,
      seededFps: persistedFps,
    });
  }

  const errors = useMemo(() => validateCanvas({ w, h, fps }), [w, h, fps]);
  const dirty = w !== composer.canvas.w || h !== composer.canvas.h || fps !== persistedFps;
  const canApply = dirty && !hasCanvasErrors(errors) && !saving;

  const handleApply = async () => {
    setSaving(true);
    try {
      // Send fps only when it diverges from the daemon default — keeps
      // the persisted TOML clean for the common case.
      const canvas =
        fps === DEFAULT_CANVAS_FPS ? { w, h } : { w, h, fps };
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
      seededW: composer.canvas.w,
      seededH: composer.canvas.h,
      seededFps: persistedFps,
    });
  };

  return (
    <Card>
      <Card.Header>
        <h2 className="text-sm font-semibold text-fg">Canvas</h2>
      </Card.Header>
      <Card.Content>
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
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
        </div>
        <div className="mt-4 flex items-center justify-between gap-3">
          {dirty ? (
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
