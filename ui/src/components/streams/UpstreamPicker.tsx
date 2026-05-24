import { useMemo } from 'react';
import { Select } from '../Select';
import { useStreamStore } from '../../hooks/useStreamStore';

// Stub for U5's UpstreamPicker. Drops a typeahead combobox in favor of a
// plain grouped <select> over `source:*` and `composer:*` refs sourced
// from the store. When U5 lands, this file is superseded by the primitive
// from `ui/src/components/primitives/UpstreamPicker.tsx`.

interface UpstreamPickerProps {
  value: string;
  onChange: (next: string) => void;
  disabled?: boolean | undefined;
  error?: string | undefined;
  required?: boolean | undefined;
}

export function UpstreamPicker({
  value,
  onChange,
  disabled,
  error,
  required,
}: Readonly<UpstreamPickerProps>) {
  const streamsById = useStreamStore((s) => s.streamsById);

  // U2/U6/U8 will add `useSourceStore` / `useComposerStore`. Until then we
  // infer candidate refs from existing streams: any current upstream value
  // gets surfaced so the picker round-trips an edit-mode selection.
  const { sourceRefs, composerRefs } = useMemo(() => {
    const sources = new Set<string>();
    const composers = new Set<string>();
    for (const s of Object.values(streamsById)) {
      // Old shape has no upstream field; fall back to nothing in that case.
      const up = (s as { upstream?: string }).upstream;
      if (!up) continue;
      if (up.startsWith('source:')) sources.add(up);
      else if (up.startsWith('composer:')) composers.add(up);
    }
    if (value.startsWith('source:')) sources.add(value);
    else if (value.startsWith('composer:')) composers.add(value);
    const cmp = (a: string, b: string) => a.localeCompare(b);
    return {
      sourceRefs: [...sources].sort(cmp),
      composerRefs: [...composers].sort(cmp),
    };
  }, [streamsById, value]);

  const errorProps = error ? { error } : {};

  return (
    <Select
      label="Upstream"
      required={required}
      value={value}
      onChange={(e) => onChange(e.target.value)}
      disabled={disabled}
      hint="source:<id> for a single capture, composer:<id> for a composed scene"
      {...errorProps}
    >
      <option value="">Select upstream...</option>
      {sourceRefs.length > 0 && (
        <optgroup label="Sources">
          {sourceRefs.map((ref) => (
            <option key={ref} value={ref}>
              {ref}
            </option>
          ))}
        </optgroup>
      )}
      {composerRefs.length > 0 && (
        <optgroup label="Composers">
          {composerRefs.map((ref) => (
            <option key={ref} value={ref}>
              {ref}
            </option>
          ))}
        </optgroup>
      )}
    </Select>
  );
}
