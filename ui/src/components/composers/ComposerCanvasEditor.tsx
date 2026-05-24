import { useEffect, useMemo, useState } from 'react';
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

// Canvas dim + fps editor with explicit Apply. Changing any of these
// fields restarts the videonode-composer process (and drops downstream
// encoders momentarily), so we don't auto-save on every keystroke the
// way the layout editor does.
export function ComposerCanvasEditor({ composer }: Readonly<ComposerCanvasEditorProps>) {
  const updateComposer = useComposerStore((s) => s.updateComposer);
  const persistedFps = composer.canvas.fps ?? DEFAULT_CANVAS_FPS;

  const [w, setW] = useState(composer.canvas.w);
  const [h, setH] = useState(composer.canvas.h);
  const [fps, setFps] = useState(persistedFps);
  const [saving, setSaving] = useState(false);

  // Reseed local state if the composer changes underneath us (SSE,
  // another tab, etc.). Comparing by primitive avoids resetting while
  // the user is typing.
  useEffect(() => {
    setW(composer.canvas.w);
    setH(composer.canvas.h);
    setFps(persistedFps);
  }, [composer.canvas.w, composer.canvas.h, persistedFps]);

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
    setW(composer.canvas.w);
    setH(composer.canvas.h);
    setFps(persistedFps);
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
            onChange={(e) => setW(parseInt(e.target.value, 10) || 0)}
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
            onChange={(e) => setH(parseInt(e.target.value, 10) || 0)}
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
            onChange={(e) => setFps(parseInt(e.target.value, 10) || 0)}
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
