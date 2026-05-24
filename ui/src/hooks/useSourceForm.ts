import { useCallback, useMemo, useState } from 'react';
import { useSourceStore } from './useSourceStore';
import type { SourceData, SourceRequestData } from './slices/types';
import type { components } from '../lib/api.generated';

type SourceFormatBody = components['schemas']['SourceFormatBody'];

function formatsEqual(
  a: SourceFormatBody | null | undefined,
  b: SourceFormatBody | null | undefined,
): boolean {
  if (!a && !b) return true;
  if (!a || !b) return false;
  return (
    a.format_name === b.format_name &&
    a.width === b.width &&
    a.height === b.height &&
    (a.fps ?? 0) === (b.fps ?? 0)
  );
}

// formatComplete is true for a fully-resolved format selection (the
// cascade picked w/h). Partial formats are kept in form state mid-edit
// but never sent to the API.
function formatComplete(f: SourceFormatBody | null | undefined): f is SourceFormatBody {
  return !!f && f.width > 0 && f.height > 0;
}

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
  // device path -> owning source id, excluding the edited source itself
  existingDevices: Map<string, string>;
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
  if (!inputs.testMode) {
    if (!inputs.device.trim()) {
      e.device = 'Device is required when test mode is off';
    } else {
      const owner = inputs.existingDevices.get(inputs.device);
      if (owner) {
        e.device = `Device is already used by source "${owner}"`;
      }
    }
  }
  return e;
}

export function useSourceForm(initialData?: SourceData) {
  const mode = initialData ? ('edit' as const) : ('create' as const);

  const [id, setId] = useState(initialData?.id ?? '');
  const [device, setDeviceState] = useState(initialData?.device ?? '');
  const [testMode, setTestMode] = useState<boolean>(initialData?.test_mode ?? false);
  const [format, setFormat] = useState<SourceFormatBody | null>(
    initialData?.format ?? null,
  );

  const setDevice = useCallback((next: string) => {
    setDeviceState((prev) => {
      if (prev !== next) setFormat(null);
      return next;
    });
  }, []);

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

  const existingDevices = useMemo(() => {
    const map = new Map<string, string>();
    for (const s of Object.values(sourcesById)) {
      if (initialData && s.id === initialData.id) continue;
      if (s.test_mode) continue;
      if (s.device) map.set(s.device, s.id);
    }
    return map;
  }, [sourcesById, initialData]);

  const errors = useMemo(
    () => validate({ mode, id, testMode, device, existingIds, existingDevices }),
    [mode, id, testMode, device, existingIds, existingDevices],
  );

  const isValid = Object.keys(errors).length === 0;

  const isDirty = useMemo(() => {
    if (mode === 'create') return true;
    if (!initialData) return false;
    return (
      device !== initialData.device ||
      testMode !== initialData.test_mode ||
      !formatsEqual(format, initialData.format)
    );
  }, [mode, initialData, device, testMode, format]);

  const toggleTestMode = useCallback(
    (next: boolean) => {
      setTestMode(next);
      if (next) {
        setDevice('');
        setFormat(null);
      }
    },
    [setDevice],
  );

  const buildRequest = useCallback((): SourceRequestData => {
    if (testMode) {
      return { id, test_mode: true, device: '' };
    }
    const base: SourceRequestData = { id, device, test_mode: false };
    if (formatComplete(format)) base.format = format;
    return base;
  }, [id, device, testMode, format]);

  const buildPatch = useCallback(
    (prev: SourceData): Partial<SourceRequestData> => {
      const payload: Partial<SourceRequestData> = {};
      if (device !== prev.device) payload.device = testMode ? '' : device;
      if (testMode !== prev.test_mode) payload.test_mode = testMode;
      if (!testMode && formatComplete(format) && !formatsEqual(format, prev.format)) {
        payload.format = format;
      }
      return payload;
    },
    [device, testMode, format],
  );

  const submit = useCallback(async (): Promise<boolean> => {
    if (!isValid) return false;
    setSaving(true);
    setError(null);
    try {
      if (mode === 'create') {
        await createSource(buildRequest());
      } else if (initialData) {
        const payload = buildPatch(initialData);
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
  }, [isValid, mode, initialData, buildRequest, buildPatch, createSource, updateSource]);

  return {
    mode,
    id,
    setId,
    device,
    setDevice,
    testMode,
    toggleTestMode,
    format,
    setFormat,
    errors,
    isValid,
    isDirty,
    saving,
    error,
    submit,
  };
}
