import { TrashIcon, PencilSquareIcon } from '@heroicons/react/24/outline';
import { Button } from '../Button';
import type { ComposerInput } from '../../hooks/useComposerStore';

interface InputsListProps {
  inputs: ComposerInput[];
  editingRef: string | null;
  disabled?: boolean;
  onEdit: (ref: string) => void;
  onRemove: (ref: string) => Promise<void> | void;
}

function effectSummary(input: ComposerInput): string {
  const effect = input.effect;
  if (!effect) return 'No effect';
  if (effect.type === 'perspective') {
    return effect.corners ? 'Perspective (4 corners)' : 'Perspective (no corners)';
  }
  return effect.type;
}

export function InputsList({
  inputs,
  editingRef,
  disabled,
  onEdit,
  onRemove,
}: Readonly<InputsListProps>) {
  if (inputs.length === 0) {
    return (
      <div className="rounded-md border border-dashed border-border bg-surface-sunken p-6 text-center text-sm text-fg-subtle">
        No inputs yet. Add a source above to build the composer scene.
      </div>
    );
  }

  return (
    <ul className="divide-y divide-border rounded-md border border-border bg-surface">
      {inputs.map((input) => {
        const isEditing = editingRef === input.ref;
        return (
          <li
            key={input.ref}
            className="flex items-center justify-between gap-3 px-4 py-3"
          >
            <div className="min-w-0 flex-1">
              <div className="truncate font-mono text-sm text-fg">{input.ref}</div>
              <div className="mt-0.5 text-xs text-fg-muted">{effectSummary(input)}</div>
            </div>
            <div className="flex items-center gap-2">
              <Button
                theme={isEditing ? 'primary' : 'light'}
                size="SM"
                text={isEditing ? 'Editing' : 'Edit'}
                LeadingIcon={PencilSquareIcon}
                onClick={() => onEdit(input.ref)}
                disabled={disabled}
              />
              <Button
                theme="danger"
                size="SM"
                text="Remove"
                LeadingIcon={TrashIcon}
                onClick={() => onRemove(input.ref)}
                disabled={disabled}
              />
            </div>
          </li>
        );
      })}
    </ul>
  );
}
