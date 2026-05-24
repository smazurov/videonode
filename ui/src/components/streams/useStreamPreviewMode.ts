import { useCallback, useState } from 'react';

export type StreamPreviewMode = 'large' | 'small' | 'off';

const STORAGE_KEY = 'videonode:stream-preview-mode';
const DEFAULT_MODE: StreamPreviewMode = 'large';

function isMode(value: unknown): value is StreamPreviewMode {
  return value === 'large' || value === 'small' || value === 'off';
}

function loadModeMap(): Record<string, StreamPreviewMode> {
  if (typeof window === 'undefined') return {};
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) return {};
    const parsed: unknown = JSON.parse(raw);
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return {};
    const out: Record<string, StreamPreviewMode> = {};
    for (const [k, v] of Object.entries(parsed as Record<string, unknown>)) {
      if (isMode(v)) out[k] = v;
    }
    return out;
  } catch {
    return {};
  }
}

function persistModeMap(map: Record<string, StreamPreviewMode>): void {
  if (typeof window === 'undefined') return;
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(map));
  } catch {
    // ignore quota / privacy-mode errors
  }
}

export function useStreamPreviewMode(streamId: string): {
  mode: StreamPreviewMode;
  setMode: (mode: StreamPreviewMode) => void;
} {
  const [trackedId, setTrackedId] = useState(streamId);
  const [mode, setModeState] = useState<StreamPreviewMode>(
    () => loadModeMap()[streamId] ?? DEFAULT_MODE,
  );

  if (trackedId !== streamId) {
    setTrackedId(streamId);
    setModeState(loadModeMap()[streamId] ?? DEFAULT_MODE);
  }

  const setMode = useCallback(
    (next: StreamPreviewMode) => {
      setModeState(next);
      const map = loadModeMap();
      if (next === DEFAULT_MODE) delete map[streamId];
      else map[streamId] = next;
      persistModeMap(map);
    },
    [streamId],
  );

  return { mode, setMode };
}
