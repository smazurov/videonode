import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { components } from '../lib/api.generated';
import { api, unwrap } from '../lib/api';
import { useStreamStore } from './useStreamStore';

type StreamData = components['schemas']['StreamData'];
type StreamRequestData = components['schemas']['StreamRequestData'];
type CanvasData = components['schemas']['CanvasData'];
type CanvasSourceOverrideData = components['schemas']['CanvasSourceOverrideData'];
type CanvasLayoutData = components['schemas']['CanvasLayoutData'];

export type RotationOverride = null | 0 | 90 | 180 | 270;

export type CanvasPreset = '1080p' | '4k';

export const CANVAS_PRESETS: Record<CanvasPreset, { width: 1920 | 3840; height: 1080 | 2160; label: string }> = {
  '1080p': { width: 1920, height: 1080, label: '1080p (1920×1080)' },
  '4k': { width: 3840, height: 2160, label: '4k (3840×2160)' },
};

const FPS_OPTIONS = ['24', '25', '30', '50', '60'];

function presetFromCanvas(canvas: CanvasData): CanvasPreset {
  if (canvas.width === 3840 && canvas.height === 2160) return '4k';
  return '1080p';
}

function dropAt<T>(arr: T[], index: number): T[] {
  const next = [...arr];
  next.splice(index, 1);
  return next;
}

export function useCanvasForm(initialData?: StreamData) {
  const mode = initialData ? ('edit' as const) : ('create' as const);
  const existingCanvas = initialData?.canvas;

  const [streamId, setStreamId] = useState(initialData?.stream_id ?? '');
  const [preset, setPreset] = useState<CanvasPreset>(
    existingCanvas ? presetFromCanvas(existingCanvas) : '1080p',
  );
  const [fps, setFps] = useState<string>(existingCanvas?.fps ?? '30');
  const [keyColor, setKeyColor] = useState<string>(existingCanvas?.key_color ?? '0x000000');
  const [sourceIds, setSourceIds] = useState<string[]>(() => existingCanvas?.source_streams ?? []);
  const [rotationOverrides, setRotationOverrides] = useState<RotationOverride[]>(() => {
    const count = (existingCanvas?.source_streams ?? []).length;
    const initial = Array.from({ length: count }, () => null as RotationOverride);
    const existing = existingCanvas?.source_overrides ?? [];
    for (const [i, ov] of existing.entries()) {
      if (!ov || i >= initial.length) continue;
      const r = ov.rotation;
      if (r === 0 || r === 90 || r === 180 || r === 270) initial[i] = r;
    }
    return initial;
  });
  const [layout, setLayout] = useState<CanvasLayoutData | null>(null);
  const [layoutLoading, setLayoutLoading] = useState(false);
  const [layoutName, setLayoutName] = useState<string>(existingCanvas?.layout_name ?? '');
  const [audioDevice, setAudioDevice] = useState<string>(() => {
    const devs = existingCanvas?.audio_devices ?? [];
    return devs.length > 0 && typeof devs[0] === 'string' ? devs[0] : '';
  });
  const [codec, setCodec] = useState<StreamRequestData['codec']>(
    (initialData?.codec as StreamRequestData['codec']) ?? 'h264',
  );
  const [bitrate, setBitrate] = useState<number>(() => {
    if (initialData?.bitrate) {
      const n = parseFloat(initialData.bitrate.replace(/[^\d.]/g, ''));
      if (!isNaN(n) && n > 0) return n;
    }
    return 8;
  });
  const [options, setOptions] = useState<string[]>(initialData?.options ?? []);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const createStream = useStreamStore((s) => s.createStream);
  const updateStream = useStreamStore((s) => s.updateStream);
  const streamsById = useStreamStore((s) => s.streamsById);

  // Available sources: all individual streams (non-canvas, non-owned)
  const availableSources = useMemo(
    () =>
      Object.values(streamsById).filter(
        (s): s is StreamData =>
          !!s && !s.canvas && (!s.owned_by || sourceIds.includes(s.stream_id)),
      ),
    [streamsById, sourceIds],
  );

  // Load default FFmpeg options on create
  useEffect(() => {
    if (mode === 'edit') return;
    api
      .GET('/api/options')
      .then((res) => {
        const data = unwrap(res, 'Failed to load options');
        setOptions((data.options ?? []).filter((o) => o.app_default).map((o) => o.key));
      })
      .catch(() => setOptions([]));
  }, [mode]);

  const addSource = useCallback((id: string) => {
    setSourceIds((cur) => {
      if (cur.includes(id) || cur.length >= 4) return cur;
      return [...cur, id];
    });
    setRotationOverrides((cur) => (cur.length >= 4 ? cur : [...cur, null]));
  }, []);

  const removeSource = useCallback((id: string) => {
    setSourceIds((cur) => {
      const idx = cur.indexOf(id);
      if (idx < 0) return cur;
      setRotationOverrides((ov) => dropAt(ov, idx));
      return cur.filter((s) => s !== id);
    });
  }, []);

  const moveSource = useCallback((index: number, delta: -1 | 1) => {
    setSourceIds((cur) => {
      const next = [...cur];
      const target = index + delta;
      if (target < 0 || target >= next.length) return cur;
      [next[index], next[target]] = [next[target]!, next[index]!];
      return next;
    });
    setRotationOverrides((cur) => {
      const next = [...cur];
      const target = index + delta;
      if (target < 0 || target >= next.length) return cur;
      [next[index], next[target]] = [next[target]!, next[index]!];
      return next;
    });
  }, []);

  const setRotationOverride = useCallback((index: number, value: RotationOverride) => {
    setRotationOverrides((cur) => {
      if (index < 0 || index >= cur.length) return cur;
      const next = [...cur];
      next[index] = value;
      return next;
    });
  }, []);

  const errors = useMemo(() => {
    const e: Record<string, string> = {};
    if (mode === 'create') {
      if (!streamId.trim()) e.streamId = 'Stream ID is required';
      else if (!/^[\w-]+$/.test(streamId))
        e.streamId = 'Stream ID can only contain letters, numbers, dashes, underscores';
    }
    if (sourceIds.length < 1) e.sources = 'Select at least one source stream';
    if (sourceIds.length > 4) e.sources = 'At most 4 source streams';
    if (!fps) e.fps = 'FPS is required';
    if (bitrate < 0.1 || bitrate > 100)
      e.bitrate = 'Bitrate must be between 0.1 and 100 Mbps';
    return e;
  }, [mode, streamId, sourceIds, fps, bitrate]);

  const isValid = Object.keys(errors).length === 0;

  const buildCanvas = useCallback((): CanvasData => {
    const { width, height } = CANVAS_PRESETS[preset];
    const overrides: CanvasSourceOverrideData[] = rotationOverrides.map((r) => ({
      ...(r !== null ? { rotation: r } : {}),
    }));
    const hasAnyOverride = rotationOverrides.some((r) => r !== null);
    return {
      width,
      height,
      fps,
      key_color: keyColor,
      source_streams: sourceIds,
      audio_devices: audioDevice ? [audioDevice] : [],
      ...(hasAnyOverride ? { source_overrides: overrides } : {}),
      ...(layoutName ? { layout_name: layoutName } : {}),
    };
  }, [preset, fps, keyColor, sourceIds, audioDevice, rotationOverrides, layoutName]);

  // Debounced canvas layout preview — backend is the single source of truth
  // for slot geometry and letterbox content rects.
  const layoutTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const layoutSeqRef = useRef(0);
  useEffect(() => {
    if (sourceIds.length === 0) {
      setLayout(null);
      setLayoutLoading(false);
      if (layoutTimerRef.current) clearTimeout(layoutTimerRef.current);
      return;
    }
    if (layoutTimerRef.current) clearTimeout(layoutTimerRef.current);
    const { width, height } = CANVAS_PRESETS[preset];
    const overrides: CanvasSourceOverrideData[] = rotationOverrides.map((r) => ({
      ...(r !== null ? { rotation: r } : {}),
    }));
    const hasAnyOverride = rotationOverrides.some((r) => r !== null);
    const canvas: CanvasData = {
      width,
      height,
      fps: '30',
      source_streams: sourceIds,
      ...(hasAnyOverride ? { source_overrides: overrides } : {}),
      ...(layoutName ? { layout_name: layoutName } : {}),
    };
    const seq = ++layoutSeqRef.current;
    setLayoutLoading(true);
    layoutTimerRef.current = setTimeout(() => {
      api
        .POST('/api/streams/canvas/layout', { body: canvas })
        .then(({ data, error: apiError }) => {
          if (seq !== layoutSeqRef.current) return;
          if (apiError || !data) {
            setLayout(null);
          } else {
            setLayout(data);
          }
        })
        .catch(() => {
          if (seq === layoutSeqRef.current) setLayout(null);
        })
        .finally(() => {
          if (seq === layoutSeqRef.current) setLayoutLoading(false);
        });
    }, 200);
    return () => {
      if (layoutTimerRef.current) clearTimeout(layoutTimerRef.current);
    };
  }, [sourceIds, rotationOverrides, preset, layoutName]);

  // Named candidates are n-specific (e.g. "2x2" only for n=4); reset on source-count change.
  const sourceCount = sourceIds.length;
  const lastCountRef = useRef(sourceCount);
  useEffect(() => {
    if (lastCountRef.current !== sourceCount) {
      lastCountRef.current = sourceCount;
      setLayoutName('');
    }
  }, [sourceCount]);

  const cycleLayout = useCallback(() => {
    const available = layout?.available_layouts ?? [];
    if (available.length <= 1) return;
    const current = layoutName || layout?.chosen_layout || available[0]!;
    const idx = available.indexOf(current);
    const next = available[(idx + 1) % available.length] ?? available[0]!;
    setLayoutName(next);
  }, [layout, layoutName]);

  const submit = useCallback(async () => {
    if (!isValid) return false;
    setSaving(true);
    setError(null);
    try {
      const canvas = buildCanvas();
      if (mode === 'create') {
        const req: StreamRequestData = {
          stream_id: streamId,
          codec,
          bitrate,
          canvas,
          ...(options.length > 0 ? { options } : {}),
        } as StreamRequestData;
        await createStream(req);
      } else {
        await updateStream(streamId, {
          canvas,
          codec,
          bitrate,
          options,
        } as Partial<StreamRequestData>);
      }
      return true;
    } catch (error_) {
      setError(error_ instanceof Error ? error_.message : 'Failed to save canvas');
      return false;
    } finally {
      setSaving(false);
    }
  }, [
    isValid,
    mode,
    streamId,
    codec,
    bitrate,
    options,
    buildCanvas,
    createStream,
    updateStream,
  ]);

  return {
    mode,
    streamId,
    setStreamId,
    preset,
    setPreset,
    fps,
    setFps,
    fpsOptions: FPS_OPTIONS,
    keyColor,
    setKeyColor,
    sourceIds,
    rotationOverrides,
    setRotationOverride,
    layout,
    layoutLoading,
    layoutName,
    chosenLayout: layout?.chosen_layout ?? '',
    availableLayouts: layout?.available_layouts ?? [],
    cycleLayout,
    addSource,
    removeSource,
    moveSource,
    availableSources,
    audioDevice,
    setAudioDevice,
    codec,
    setCodec,
    bitrate,
    setBitrate,
    options,
    setOptions,
    errors,
    isValid,
    saving,
    error,
    submit,
    canvasDimensions: CANVAS_PRESETS[preset],
  };
}
