import { useEffect, useMemo } from 'react';
import { Select } from '../Select';
import { useSourceStore } from '../../hooks/useSourceStore';
import { useComposerStore } from '../../hooks/useComposerStore';

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
  const sourcesById = useSourceStore((s) => s.sourcesById);
  const sourcesLastUpdated = useSourceStore((s) => s.lastUpdated);
  const fetchSources = useSourceStore((s) => s.fetchSources);

  const composersById = useComposerStore((s) => s.composersById);
  const composersLastUpdated = useComposerStore((s) => s.lastUpdated);
  const fetchComposers = useComposerStore((s) => s.fetchComposers);

  useEffect(() => {
    if (sourcesLastUpdated === null) void fetchSources();
  }, [sourcesLastUpdated, fetchSources]);

  useEffect(() => {
    if (composersLastUpdated === null) void fetchComposers();
  }, [composersLastUpdated, fetchComposers]);

  const { sourceRefs, composerRefs } = useMemo(() => {
    const sources = new Set<string>();
    for (const id of Object.keys(sourcesById)) sources.add(`source:${id}`);
    const composers = new Set<string>();
    for (const id of Object.keys(composersById)) composers.add(`composer:${id}`);
    // Surface the current value even if its upstream entity was deleted,
    // so the picker round-trips an edit-mode selection.
    if (value.startsWith('source:')) sources.add(value);
    else if (value.startsWith('composer:')) composers.add(value);
    const cmp = (a: string, b: string) => a.localeCompare(b);
    return {
      sourceRefs: [...sources].sort(cmp),
      composerRefs: [...composers].sort(cmp),
    };
  }, [sourcesById, composersById, value]);

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
