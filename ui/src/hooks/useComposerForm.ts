import { useCallback, useMemo, useState } from 'react';
import { API_BASE_URL } from '../lib/api';
import { getAuthCredentials } from '../lib/auth';

// Local stubs of backend types until /api/composers schema regen lands.
// Integrators: swap these for `components['schemas']['ComposerData']` &c.
export type ComposerCanvasDims = { w: number; h: number; fps?: number };

// Daemon-side default canvas frame rate (pipeline.DefaultCanvasFPS).
export const DEFAULT_CANVAS_FPS = 60;

export type ComposerLayoutSlot = {
  input: string;
  x: number;
  y: number;
  w: number;
  h: number;
};

export type ComposerInputRef = {
  ref: string;
};

export type ComposerCreateRequest = {
  id: string;
  canvas: ComposerCanvasDims;
  inputs: ComposerInputRef[];
  layout: ComposerLayoutSlot[];
};

export type CanvasPresetKey = '1080p' | '1440p' | '4k' | 'custom';

export const CANVAS_PRESETS: Record<
  Exclude<CanvasPresetKey, 'custom'>,
  { w: number; h: number; label: string }
> = {
  '1080p': { w: 1920, h: 1080, label: '1920 × 1080' },
  '1440p': { w: 2560, h: 1440, label: '2560 × 1440' },
  '4k': { w: 3840, h: 2160, label: '3840 × 2160' },
};

const ID_PATTERN = /^[\da-z][\da-z-]*$/;

export type WizardStep = 'identity' | 'inputs' | 'layout';

export interface UseComposerFormResult {
  step: WizardStep;
  goNext: () => void;
  goBack: () => void;
  setStep: (s: WizardStep) => void;

  composerId: string;
  setComposerId: (id: string) => void;
  preset: CanvasPresetKey;
  setPreset: (p: CanvasPresetKey) => void;
  customW: number;
  setCustomW: (n: number) => void;
  customH: number;
  setCustomH: (n: number) => void;
  fps: number;
  setFps: (n: number) => void;
  canvas: ComposerCanvasDims;

  selectedSourceIds: string[];
  toggleSource: (id: string) => void;
  clearSources: () => void;

  layout: ComposerLayoutSlot[];

  errors: Record<string, string>;
  stepValid: Record<WizardStep, boolean>;
  isValid: boolean;
  saving: boolean;
  error: string | null;

  submit: () => Promise<boolean>;
  request: ComposerCreateRequest;
}

interface UseComposerFormOptions {
  existingIds?: string[];
}

// Auto-suggest layouts for 1..N inputs. Single → full-screen, two →
// side-by-side, three → 2-up top + 1-wide bottom, four → 2×2 grid,
// 5+ → 3×ceil(N/3) grid. Slots laid out left-to-right, top-to-bottom.
export function suggestLayout(
  canvas: ComposerCanvasDims,
  inputRefs: string[],
): ComposerLayoutSlot[] {
  const n = inputRefs.length;
  if (n === 0) return [];
  if (n === 1) {
    return [{ input: inputRefs[0]!, x: 0, y: 0, w: canvas.w, h: canvas.h }];
  }
  if (n === 2) {
    const halfW = Math.floor(canvas.w / 2);
    return [
      { input: inputRefs[0]!, x: 0, y: 0, w: halfW, h: canvas.h },
      { input: inputRefs[1]!, x: halfW, y: 0, w: canvas.w - halfW, h: canvas.h },
    ];
  }
  if (n === 3) {
    const halfW = Math.floor(canvas.w / 2);
    const halfH = Math.floor(canvas.h / 2);
    return [
      { input: inputRefs[0]!, x: 0, y: 0, w: halfW, h: halfH },
      { input: inputRefs[1]!, x: halfW, y: 0, w: canvas.w - halfW, h: halfH },
      { input: inputRefs[2]!, x: 0, y: halfH, w: canvas.w, h: canvas.h - halfH },
    ];
  }
  if (n === 4) {
    const halfW = Math.floor(canvas.w / 2);
    const halfH = Math.floor(canvas.h / 2);
    return [
      { input: inputRefs[0]!, x: 0, y: 0, w: halfW, h: halfH },
      { input: inputRefs[1]!, x: halfW, y: 0, w: canvas.w - halfW, h: halfH },
      { input: inputRefs[2]!, x: 0, y: halfH, w: halfW, h: canvas.h - halfH },
      { input: inputRefs[3]!, x: halfW, y: halfH, w: canvas.w - halfW, h: canvas.h - halfH },
    ];
  }
  const cols = 3;
  const rows = Math.ceil(n / cols);
  const cellW = Math.floor(canvas.w / cols);
  const cellH = Math.floor(canvas.h / rows);
  return inputRefs.map((ref, i) => {
    const col = i % cols;
    const row = Math.floor(i / cols);
    const w = col === cols - 1 ? canvas.w - cellW * col : cellW;
    const h = row === rows - 1 ? canvas.h - cellH * row : cellH;
    return { input: ref, x: cellW * col, y: cellH * row, w, h };
  });
}

const STEPS: WizardStep[] = ['identity', 'inputs', 'layout'];

export function useComposerForm(options: UseComposerFormOptions = {}): UseComposerFormResult {
  const optionExistingIds = options.existingIds;
  const existingIds = useMemo(() => optionExistingIds ?? [], [optionExistingIds]);

  const [step, setStep] = useState<WizardStep>('identity');
  const [composerId, setComposerId] = useState('');
  const [preset, setPreset] = useState<CanvasPresetKey>('1080p');
  const [customW, setCustomW] = useState(1920);
  const [customH, setCustomH] = useState(1080);
  const [fps, setFps] = useState<number>(DEFAULT_CANVAS_FPS);
  const [selectedSourceIds, setSelectedSourceIds] = useState<string[]>([]);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const canvas: ComposerCanvasDims = useMemo(() => {
    const dims = preset === 'custom'
      ? { w: customW, h: customH }
      : { w: CANVAS_PRESETS[preset].w, h: CANVAS_PRESETS[preset].h };
    // Persist fps only when the user picks something other than the
    // daemon default — keeps round-tripped TOML clean for the common
    // case where the operator didn't override.
    return fps === DEFAULT_CANVAS_FPS ? dims : { ...dims, fps };
  }, [preset, customW, customH, fps]);

  const inputRefs = useMemo(
    () => selectedSourceIds.map((id) => `source:${id}`),
    [selectedSourceIds],
  );

  const layout = useMemo(() => suggestLayout(canvas, inputRefs), [canvas, inputRefs]);

  const errors = useMemo(() => {
    const e: Record<string, string> = {};
    if (!composerId.trim()) {
      e.composerId = 'Composer ID is required';
    } else if (!ID_PATTERN.test(composerId)) {
      e.composerId = 'Lowercase letters, digits, dashes (start with letter or digit)';
    } else if (existingIds.includes(composerId)) {
      e.composerId = 'A composer with this ID already exists';
    }
    if (preset === 'custom') {
      if (customW < 16 || customW > 7680) e.customW = 'Width must be 16-7680';
      if (customH < 16 || customH > 4320) e.customH = 'Height must be 16-4320';
      if (customW % 2 !== 0) e.customW = 'Width must be even';
      if (customH % 2 !== 0) e.customH = 'Height must be even';
    }
    if (!Number.isFinite(fps) || fps < 1 || fps > 240) {
      e.fps = 'Frame rate must be 1-240';
    }
    if (selectedSourceIds.length === 0) {
      e.inputs = 'Pick at least one source';
    }
    return e;
  }, [composerId, preset, customW, customH, fps, selectedSourceIds, existingIds]);

  const stepValid: Record<WizardStep, boolean> = useMemo(
    () => ({
      identity: !errors.composerId && !errors.customW && !errors.customH && !errors.fps,
      inputs: !errors.inputs,
      layout: true,
    }),
    [errors],
  );

  const isValid = Object.keys(errors).length === 0;

  const toggleSource = useCallback((id: string) => {
    setSelectedSourceIds((cur) =>
      cur.includes(id) ? cur.filter((s) => s !== id) : [...cur, id],
    );
  }, []);

  const clearSources = useCallback(() => setSelectedSourceIds([]), []);

  const goNext = useCallback(() => {
    setStep((cur) => {
      const idx = STEPS.indexOf(cur);
      if (idx < 0 || idx >= STEPS.length - 1) return cur;
      return STEPS[idx + 1]!;
    });
  }, []);

  const goBack = useCallback(() => {
    setStep((cur) => {
      const idx = STEPS.indexOf(cur);
      if (idx <= 0) return cur;
      return STEPS[idx - 1]!;
    });
  }, []);

  const request: ComposerCreateRequest = useMemo(
    () => ({
      id: composerId,
      canvas,
      inputs: inputRefs.map((ref) => ({ ref })),
      layout,
    }),
    [composerId, canvas, inputRefs, layout],
  );

  const submit = useCallback(async () => {
    if (!isValid) return false;
    setSaving(true);
    setError(null);
    try {
      // POST /api/composers — endpoint not yet in api.generated.ts so we use
      // plain fetch with the same auth header openapi-fetch's middleware sets.
      const credentials = getAuthCredentials();
      const headers: Record<string, string> = {
        'Content-Type': 'application/json',
      };
      if (credentials) headers.Authorization = `Basic ${credentials}`;
      const response = await fetch(`${API_BASE_URL}/api/composers`, {
        method: 'POST',
        headers,
        body: JSON.stringify(request),
      });
      if (!response.ok) {
        const text = await response.text().catch(() => '');
        throw new Error(text || `Failed to create composer (${response.status})`);
      }
      return true;
    } catch (error_) {
      setError(error_ instanceof Error ? error_.message : 'Failed to create composer');
      return false;
    } finally {
      setSaving(false);
    }
  }, [isValid, request]);

  return {
    step,
    goNext,
    goBack,
    setStep,
    composerId,
    setComposerId,
    preset,
    setPreset,
    customW,
    setCustomW,
    customH,
    setCustomH,
    fps,
    setFps,
    canvas,
    selectedSourceIds,
    toggleSource,
    clearSources,
    layout,
    errors,
    stepValid,
    isValid,
    saving,
    error,
    submit,
    request,
  };
}
