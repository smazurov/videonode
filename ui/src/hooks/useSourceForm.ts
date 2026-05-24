import { useCallback, useMemo, useState } from 'react';
import { useSourceStore } from './useSourceStore';
import type { SourceData, SourceRequestData } from './slices/types';

// Manual kebab-case check — avoids the security/detect-unsafe-regex lint
// trip from anchored `^([a-z0-9]+)(-[a-z0-9]+)*$`.
function isKebabCase(s: string): boolean {
  if (s.length === 0) return false;
  if (s.startsWith('-') || s.endsWith('-')) return false;
  let prevDash = false;
  for (const ch of s) {
    const isLower = ch >= 'a' && ch <= 'z';
    const isDigit = ch >= '0' && ch <= '9';
    const isDash = ch === '-';
    if (!isLower && !isDigit && !isDash) return false;
    if (isDash && prevDash) return false;
    prevDash = isDash;
  }
  return true;
}

interface ErrorInputs {
  mode: 'create' | 'edit';
  id: string;
  testMode: boolean;
  device: string;
  existingIds: Set<string>;
}

function validate(inputs: ErrorInputs): Record<string, string> {
  const e: Record<string, string> = {};
  if (inputs.mode === 'create') {
    if (!inputs.id.trim()) {
      e.id = 'Source ID is required';
    } else if (!isKebabCase(inputs.id)) {
      e.id = 'Source ID must be kebab-case (lowercase, digits, dashes)';
    } else if (inputs.existingIds.has(inputs.id)) {
      e.id = `A source with id "${inputs.id}" already exists`;
    }
  }
  if (!inputs.testMode && !inputs.device.trim()) {
    e.device = 'Device is required when test mode is off';
  }
  return e;
}

export function useSourceForm(initialData?: SourceData) {
  const mode = initialData ? ('edit' as const) : ('create' as const);

  const [id, setId] = useState(initialData?.id ?? '');
  const [device, setDevice] = useState(initialData?.device ?? '');
  const [testMode, setTestMode] = useState<boolean>(initialData?.test_mode ?? false);

  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const createSource = useSourceStore((s) => s.createSource);
  const updateSource = useSourceStore((s) => s.updateSource);
  const sourcesById = useSourceStore((s) => s.sourcesById);

  const existingIds = useMemo(() => {
    const set = new Set<string>();
    for (const s of Object.values(sourcesById)) {
      if (initialData && s.id === initialData.id) continue;
      set.add(s.id);
    }
    return set;
  }, [sourcesById, initialData]);

  const errors = useMemo(
    () => validate({ mode, id, testMode, device, existingIds }),
    [mode, id, testMode, device, existingIds],
  );

  const isValid = Object.keys(errors).length === 0;

  const isDirty = useMemo(() => {
    if (mode === 'create') return true;
    if (!initialData) return false;
    return (
      device !== initialData.device ||
      testMode !== initialData.test_mode
    );
  }, [mode, initialData, device, testMode]);

  const toggleTestMode = useCallback((next: boolean) => {
    setTestMode(next);
    if (next) setDevice('');
  }, []);

  const buildRequest = useCallback((): SourceRequestData => {
    if (testMode) {
      return { id, test_mode: true, device: '' };
    }
    return { id, device, test_mode: false };
  }, [id, device, testMode]);

  const submit = useCallback(async (): Promise<boolean> => {
    if (!isValid) return false;
    setSaving(true);
    setError(null);
    try {
      if (mode === 'create') {
        await createSource(buildRequest());
      } else if (initialData) {
        const payload: Partial<SourceRequestData> = {};
        if (device !== initialData.device) payload.device = testMode ? '' : device;
        if (testMode !== initialData.test_mode) payload.test_mode = testMode;
        if (Object.keys(payload).length > 0) {
          await updateSource(initialData.id, payload);
        }
      }
      return true;
    } catch (error_) {
      setError(error_ instanceof Error ? error_.message : 'Failed to save source');
      return false;
    } finally {
      setSaving(false);
    }
  }, [
    isValid,
    mode,
    initialData,
    device,
    testMode,
    buildRequest,
    createSource,
    updateSource,
  ]);

  return {
    mode,
    id,
    setId,
    device,
    setDevice,
    testMode,
    toggleTestMode,
    errors,
    isValid,
    isDirty,
    saving,
    error,
    submit,
  };
}
