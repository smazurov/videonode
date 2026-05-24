import { useMemo, useState } from 'react';
import { Button } from '../Button';
import { Select } from '../Select';
import type { AvailableSource } from '../../hooks/useComposerStore';

interface InputRefPickerProps {
  availableSources: AvailableSource[];
  existingRefs: string[];
  disabled?: boolean;
  onAdd: (ref: string) => Promise<void> | void;
}

export function InputRefPicker({
  availableSources,
  existingRefs,
  disabled,
  onAdd,
}: Readonly<InputRefPickerProps>) {
  const candidates = useMemo(() => {
    const taken = new Set(existingRefs);
    return availableSources
      .map((s) => ({ ref: `source:${s.id}`, label: s.label ?? s.id }))
      .filter((c) => !taken.has(c.ref));
  }, [availableSources, existingRefs]);

  const [selected, setSelected] = useState<string>('');
  const [busy, setBusy] = useState(false);

  const handleAdd = async () => {
    const ref = selected || candidates[0]?.ref;
    if (!ref) return;
    setBusy(true);
    try {
      await onAdd(ref);
      setSelected('');
    } finally {
      setBusy(false);
    }
  };

  const empty = candidates.length === 0;

  return (
    <div className="flex items-end gap-2">
      <div className="flex-1">
        <Select
          label="Add input"
          value={selected}
          onChange={(e) => setSelected(e.target.value)}
          disabled={disabled || empty || busy}
        >
          {empty ? (
            <option value="">No sources available</option>
          ) : (
            <>
              <option value="">Select a source...</option>
              {candidates.map((c) => (
                <option key={c.ref} value={c.ref}>
                  {c.label} ({c.ref})
                </option>
              ))}
            </>
          )}
        </Select>
      </div>
      <Button
        theme="primary"
        size="SM"
        text={busy ? 'Adding...' : 'Add'}
        onClick={handleAdd}
        disabled={disabled || empty || busy || !selected}
      />
    </div>
  );
}
