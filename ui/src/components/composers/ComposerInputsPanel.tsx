import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import toast from 'react-hot-toast';
import { TrashIcon, PencilSquareIcon } from '@heroicons/react/24/outline';

import { Badge } from '../Badge';
import { Button } from '../Button';
import { Card } from '../Card';
import { StatusPill, type StatusPillStatus } from '../primitives/StatusPill';
import { EffectEditor } from './EffectEditor';
import { InputRefPicker } from './InputRefPicker';
import type { ComposerData, ComposerInput } from '../../lib/composer-types';
import { useComposerStore, type AvailableSource, type ComposerEffect } from '../../hooks/useComposerStore';
import { useSourceStore } from '../../hooks/useSourceStore';
import type { Source } from '../../hooks/slices/types';
import type { components } from '../../lib/api.generated';

type ComposerInputWire = components['schemas']['ComposerInputData'];

// Convert the locally-typed inputs into the wire shape expected by the
// PATCH /api/composers/{id} body. The local Effect has narrower types
// (e.g. type: 'perspective' literal); the generated EffectData is wider
// so the assignment is purely a structural widening.
function toWireInputs(inputs: ComposerInput[]): ComposerInputWire[] {
  return inputs.map((i) => {
    if (!i.effect) return { ref: i.ref };
    const effect: ComposerInputWire['effect'] = { type: i.effect.type };
    if (i.effect.corners) effect!.corners = i.effect.corners;
    return { ref: i.ref, effect };
  });
}

interface ComposerInputsPanelProps {
  composer: ComposerData;
}

const SOURCE_REF_PREFIX = 'source:';

function sourceIdFromRef(ref: string): string | null {
  if (!ref.startsWith(SOURCE_REF_PREFIX)) return null;
  return ref.slice(SOURCE_REF_PREFIX.length);
}

function resolveStatus(source: Source | undefined): StatusPillStatus | 'missing' {
  if (!source) return 'missing';
  return source.status ?? 'idle';
}

function sourceLabel(source: Source): string {
  if (source.test_mode) return `${source.id} (test pattern)`;
  return source.device ?? source.id;
}

export function ComposerInputsPanel({ composer }: Readonly<ComposerInputsPanelProps>) {
  const updateComposer = useComposerStore((s) => s.updateComposer);
  const updateInputEffect = useComposerStore((s) => s.updateComposerInputEffect);

  const sourcesById = useSourceStore((s) => s.sourcesById);
  const sourceIds = useSourceStore((s) => s.sourceIds);
  const fetchSources = useSourceStore((s) => s.fetchSources);
  const sourcesLastUpdated = useSourceStore((s) => s.lastUpdated);
  useEffect(() => {
    if (sourcesLastUpdated === null) void fetchSources();
  }, [sourcesLastUpdated, fetchSources]);

  const availableSources: AvailableSource[] = useMemo(
    () =>
      sourceIds
        .map((id) => sourcesById[id])
        .filter((s): s is Source => Boolean(s))
        .map((s) => ({ id: s.id, label: sourceLabel(s) })),
    [sourceIds, sourcesById],
  );

  const [editingRef, setEditingRef] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const handleAdd = useCallback(
    async (ref: string) => {
      setBusy(true);
      try {
        const next = [...composer.inputs, { ref }];
        await updateComposer(composer.composer_id, { inputs: toWireInputs(next) });
        toast.success(`Added ${ref}`);
      } catch (error) {
        toast.error(error instanceof Error ? error.message : 'Failed to add input');
      } finally {
        setBusy(false);
      }
    },
    [composer.composer_id, composer.inputs, updateComposer],
  );

  const handleRemove = useCallback(
    async (ref: string) => {
      setBusy(true);
      try {
        const next = composer.inputs.filter((i) => i.ref !== ref);
        await updateComposer(composer.composer_id, { inputs: toWireInputs(next) });
        if (editingRef === ref) setEditingRef(null);
        toast.success(`Removed ${ref}`);
      } catch (error) {
        toast.error(error instanceof Error ? error.message : 'Failed to remove input');
      } finally {
        setBusy(false);
      }
    },
    [composer.composer_id, composer.inputs, editingRef, updateComposer],
  );

  const handleSaveEffect = useCallback(
    async (ref: string, effect: ComposerEffect | null) => {
      setBusy(true);
      try {
        await updateInputEffect(composer.composer_id, ref, effect);
        toast.success('Effect saved');
      } catch (error) {
        toast.error(error instanceof Error ? error.message : 'Failed to save effect');
        throw error;
      } finally {
        setBusy(false);
      }
    },
    [composer.composer_id, updateInputEffect],
  );

  const existingRefs = composer.inputs.map((i) => i.ref);

  const editingInput: ComposerInput | undefined = useMemo(
    () => composer.inputs.find((i) => i.ref === editingRef),
    [editingRef, composer.inputs],
  );

  return (
    <Card padding="none">
      <div className="flex items-center justify-between border-b border-border px-4 py-3">
        <h2 className="text-sm font-semibold text-fg">Inputs</h2>
        <span className="text-xs text-fg-muted">{composer.inputs.length} total</span>
      </div>

      <div className="space-y-4 p-4">
        <InputRefPicker
          availableSources={availableSources}
          existingRefs={existingRefs}
          disabled={busy}
          onAdd={handleAdd}
        />

        {composer.inputs.length === 0 ? (
          <div className="rounded-md border border-dashed border-border bg-surface-sunken p-6 text-center text-sm text-fg-subtle">
            No inputs yet. Pick a source above to start building this scene.
          </div>
        ) : (
          <ul className="divide-y divide-border rounded-md border border-border bg-surface">
            {composer.inputs.map((input) => {
              const sid = sourceIdFromRef(input.ref);
              return (
                <InputRow
                  key={input.ref}
                  input={input}
                  source={sid ? sourcesById[sid] : undefined}
                  isEditing={editingRef === input.ref}
                  disabled={busy}
                  onEdit={() =>
                    setEditingRef((cur) => (cur === input.ref ? null : input.ref))
                  }
                  onRemove={() => void handleRemove(input.ref)}
                />
              );
            })}
          </ul>
        )}

        {editingInput && (
          <EffectEditor
            composerId={composer.composer_id}
            inputRef={editingInput.ref}
            effect={editingInput.effect ?? null}
            snapshotSourceId={sourceIdFromRef(editingInput.ref)}
            inputWidth={composer.canvas.w}
            inputHeight={composer.canvas.h}
            saving={busy}
            onSave={(effect) => handleSaveEffect(editingInput.ref, effect)}
            onCancel={() => setEditingRef(null)}
          />
        )}
      </div>
    </Card>
  );
}

interface InputRowProps {
  input: ComposerInput;
  source: Source | undefined;
  isEditing: boolean;
  disabled: boolean;
  onEdit: () => void;
  onRemove: () => void;
}

function InputRow({ input, source, isEditing, disabled, onEdit, onRemove }: Readonly<InputRowProps>) {
  const sourceId = sourceIdFromRef(input.ref);
  const status = resolveStatus(source);
  return (
    <li className="flex items-center justify-between gap-3 px-4 py-3">
      <div className="min-w-0 flex-1">
        {sourceId ? (
          <Link
            to={`/sources/${encodeURIComponent(sourceId)}`}
            className="block truncate font-mono text-sm text-accent hover:underline"
          >
            {input.ref}
          </Link>
        ) : (
          <span className="block truncate font-mono text-sm">{input.ref}</span>
        )}
        <div className="mt-1 flex items-center gap-2 text-xs text-fg-muted">
          {status === 'missing' ? (
            <Badge tone="danger" size="xs">missing</Badge>
          ) : (
            <StatusPill status={status} size="xs" />
          )}
          {input.effect ? (
            <Badge tone="info" size="xs">{input.effect.type}</Badge>
          ) : (
            <span className="text-fg-subtle">no effect</span>
          )}
        </div>
      </div>
      <div className="flex items-center gap-2">
        <Button
          theme={isEditing ? 'primary' : 'light'}
          size="SM"
          text={isEditing ? 'Editing' : 'Effect'}
          LeadingIcon={PencilSquareIcon}
          onClick={onEdit}
          disabled={disabled}
        />
        <Button
          theme="danger"
          size="SM"
          text="Remove"
          LeadingIcon={TrashIcon}
          onClick={onRemove}
          disabled={disabled}
        />
      </div>
    </li>
  );
}
