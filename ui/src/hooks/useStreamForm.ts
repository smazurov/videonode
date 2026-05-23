import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useStreamStore } from './useStreamStore';
import { useDeviceInputForm, type UseDeviceInputFormResult } from './useDeviceInputForm';
import type { components } from '../lib/api.generated';
import { api, unwrap } from '../lib/api';
import { parseResolution } from '../lib/resolution';

type StreamData = components['schemas']['StreamData'];
type StreamRequestData = components['schemas']['StreamRequestData'];
type CanvasData = components['schemas']['CanvasData'];
type CanvasLayoutData = components['schemas']['CanvasLayoutData'];
type CanvasSourceOverrideData = components['schemas']['CanvasSourceOverrideData'];

export type RotationOverride = null | 0 | 90 | 180 | 270;
export type CanvasPreset = '1080p' | '1440p' | '4k';

export const CANVAS_PRESETS: Record<
  CanvasPreset,
  { width: 1920 | 2560 | 3840; height: 1080 | 1440 | 2160; label: string }
> = {
  '1080p': { width: 1920, height: 1080, label: '1080p' },
  '1440p': { width: 2560, height: 1440, label: '1440p' },
  '4k': { width: 3840, height: 2160, label: '4k' },
};

const FPS_OPTIONS = ['24', '25', '30', '50', '60'];

function presetFromCanvas(c: CanvasData | undefined): CanvasPreset {
  if (!c) return '1080p';
  if (c.width === 3840 && c.height === 2160) return '4k';
  if (c.width === 2560 && c.height === 1440) return '1440p';
  return '1080p';
}

function dropAt<T>(arr: T[], index: number): T[] {
  const next = [...arr];
  next.splice(index, 1);
  return next;
}

interface ErrorInputs {
  mode: 'edit' | 'create';
  streamId: string;
  isMulti: boolean;
  deviceId: string;
  inputFormat: string;
  sourceIds: string[];
  fps: string;
  codec: string;
  bitrate: number;
}

function validateIdentity(e: Record<string, string>, mode: ErrorInputs['mode'], streamId: string) {
  if (mode !== 'create') return;
  if (!streamId.trim()) {
    e.streamId = 'Stream ID is required';
  } else if (!/^[\w-]+$/.test(streamId)) {
    e.streamId = 'Stream ID can only contain letters, numbers, dashes, underscores';
  }
}

function validateSingle(
  e: Record<string, string>,
  mode: ErrorInputs['mode'],
  deviceId: string,
  inputFormat: string,
) {
  if (mode !== 'create') return;
  if (!deviceId) e.deviceId = 'Device selection is required';
  if (!inputFormat) e.input_format = 'Format selection is required';
}

function validateMulti(e: Record<string, string>, sourceIds: string[], fps: string) {
  if (sourceIds.length < 1) e.sources = 'Pick at least one source stream';
  else if (sourceIds.length > 4) e.sources = 'At most 4 source streams';
  if (!fps) e.fps = 'FPS is required';
}

interface InitialSnapshot {
  preset: CanvasPreset;
  fps: string;
  keyColor: string;
  sourceIds: string[];
  audioTracks: string[];
  layoutName: string;
  codec: StreamRequestData['codec'];
  bitrate: number;
  options: string[];
  rotation: number;
}

function diffMultiEdit(
  nextCanvas: CanvasData,
  initial: InitialSnapshot,
  codec: StreamRequestData['codec'],
  bitrate: number,
  options: string[],
): Partial<StreamRequestData> {
  const a = JSON.stringify;
  const prevCanvasRel: CanvasData = {
    width: CANVAS_PRESETS[initial.preset].width,
    height: CANVAS_PRESETS[initial.preset].height,
    fps: initial.fps,
    key_color: initial.keyColor,
    source_streams: initial.sourceIds,
    audio_devices: initial.audioTracks,
    ...(initial.layoutName ? { layout_name: initial.layoutName } : {}),
  };
  const changes: Record<string, unknown> = {};
  if (a(nextCanvas) !== a(prevCanvasRel)) changes.canvas = nextCanvas;
  if (codec !== initial.codec) changes.codec = codec;
  if (bitrate !== initial.bitrate) changes.bitrate = bitrate;
  if (a(options) !== a(initial.options)) changes.options = options;
  return changes as Partial<StreamRequestData>;
}

function diffSingleEdit(
  inputDiff: Partial<StreamRequestData>,
  initial: InitialSnapshot,
  codec: StreamRequestData['codec'],
  bitrate: number,
  audioTracks: string[],
  options: string[],
  rotation: number,
): Partial<StreamRequestData> {
  const a = JSON.stringify;
  const changes: Record<string, unknown> = { ...(inputDiff as Record<string, unknown>) };
  if (codec !== initial.codec) changes.codec = codec;
  if (bitrate !== initial.bitrate) changes.bitrate = bitrate;
  const newAudio = audioTracks[0] ?? '';
  const oldAudio = initial.audioTracks[0] ?? '';
  // Only emit audio_device when the value actually changed; never send an
  // empty string as a deletion sentinel (omitempty backends drop it).
  if (newAudio !== oldAudio && newAudio !== '') changes.audio_device = newAudio;
  if (a(options) !== a(initial.options)) changes.options = options;
  if (rotation !== initial.rotation) changes.rotation = rotation;
  return changes as Partial<StreamRequestData>;
}

function computeErrors(inputs: ErrorInputs): Record<string, string> {
  const e: Record<string, string> = {};
  validateIdentity(e, inputs.mode, inputs.streamId);
  if (inputs.isMulti) validateMulti(e, inputs.sourceIds, inputs.fps);
  else validateSingle(e, inputs.mode, inputs.deviceId, inputs.inputFormat);
  if (!inputs.codec) e.codec = 'Codec selection is required';
  if (inputs.bitrate < 0.1 || inputs.bitrate > 100)
    e.bitrate = 'Bitrate must be between 0.1 and 100 Mbps';
  return e;
}

export function useStreamForm(initialData?: StreamData) {
  const mode = initialData ? ('edit' as const) : ('create' as const);
  const initialCanvas = initialData?.canvas;
  const isCanvasEdit = mode === 'edit' && !!initialCanvas;

  // sourceIds is the multi-source canvas list (refs to existing standalone streams).
  // For a single-source stream, sourceIds is empty and deviceId+inputForm describe the capture.
  const [streamId, setStreamId] = useState(initialData?.stream_id ?? '');
  const [deviceId, setDeviceId] = useState(isCanvasEdit ? '' : (initialData?.device_id ?? ''));
  const [sourceIds, setSourceIds] = useState<string[]>(
    () => (initialCanvas?.source_streams ?? []).filter((s): s is string => !!s),
  );
  const [rotationOverrides, setRotationOverrides] = useState<RotationOverride[]>(() => {
    const count = (initialCanvas?.source_streams ?? []).length;
    const out: RotationOverride[] = Array.from({ length: count }, () => null);
    const existing = initialCanvas?.source_overrides ?? [];
    for (const [i, ov] of existing.entries()) {
      if (!ov || i >= out.length) continue;
      const r = ov.rotation;
      if (r === 0 || r === 90 || r === 180 || r === 270) out[i] = r;
    }
    return out;
  });

  // Audio tracks: each entry is one output audio track in the published stream.
  const [audioTracks, setAudioTracks] = useState<string[]>(() => {
    if (initialCanvas) {
      return (initialCanvas.audio_devices ?? []).filter((d): d is string => typeof d === 'string');
    }
    return initialData?.audio_device ? [initialData.audio_device] : [];
  });

  // Single-source rotation (only meaningful when sourceIds.length === 0)
  const [rotation, setRotation] = useState<number>(initialData?.rotation ?? 0);

  // Canvas geometry
  const [preset, setPreset] = useState<CanvasPreset>(presetFromCanvas(initialCanvas));
  const [fps, setFps] = useState<string>(initialCanvas?.fps ?? '30');
  const [keyColor, setKeyColor] = useState<string>(initialCanvas?.key_color ?? '0x000000');
  const [layoutName, setLayoutName] = useState<string>(initialCanvas?.layout_name ?? '');

  // Stream-level encoder
  const [codec, setCodec] = useState<StreamRequestData['codec']>(
    (initialData?.codec as StreamRequestData['codec']) ?? 'h264',
  );
  const [bitrate, setBitrate] = useState<number>(() => {
    if (initialData?.bitrate) {
      const n = parseFloat(initialData.bitrate.replace(/[^\d.]/g, ''));
      if (!isNaN(n) && n > 0) return n;
    }
    return 2;
  });
  const [options, setOptions] = useState<string[]>(initialData?.options ?? []);

  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const createStream = useStreamStore((s) => s.createStream);
  const updateStream = useStreamStore((s) => s.updateStream);
  const streamsById = useStreamStore((s) => s.streamsById);

  const isMulti = sourceIds.length > 0;

  const initial = useMemo(() => {
    if (!initialData) return null;
    const initialSourceIds = (initialCanvas?.source_streams ?? []).filter(
      (s): s is string => !!s,
    );
    const initialRotationOverrides: RotationOverride[] = initialSourceIds.map((_, i) => {
      const r = initialCanvas?.source_overrides?.[i]?.rotation;
      if (r === 0 || r === 90 || r === 180 || r === 270) return r;
      return null;
    });
    let initialAudio: string[];
    if (initialCanvas) {
      initialAudio = (initialCanvas.audio_devices ?? []).filter(
        (d): d is string => typeof d === 'string',
      );
    } else {
      initialAudio = initialData.audio_device ? [initialData.audio_device] : [];
    }
    return {
      streamId: initialData.stream_id,
      deviceId: isCanvasEdit ? '' : (initialData.device_id ?? ''),
      sourceIds: initialSourceIds,
      rotationOverrides: initialRotationOverrides,
      audioTracks: initialAudio,
      rotation: initialData.rotation ?? 0,
      preset: presetFromCanvas(initialCanvas),
      fps: initialCanvas?.fps ?? '30',
      keyColor: initialCanvas?.key_color ?? '0x000000',
      layoutName: initialCanvas?.layout_name ?? '',
      codec: (initialData.codec as StreamRequestData['codec']) ?? 'h264',
      bitrate: initialData.bitrate
        ? parseFloat(initialData.bitrate.replace(/[^\d.]/g, '')) || 2
        : 2,
      options: initialData.options ?? [],
      inputFormat: initialData.input_format ?? '',
      ...parseResolution(initialData.resolution),
      framerate: initialData.framerate ? parseInt(initialData.framerate, 10) || 0 : 0,
    };
  }, [initialData, initialCanvas, isCanvasEdit]);

  // Single-source device-input form (format/resolution/framerate). Only meaningful when !isMulti.
  const inputForm: UseDeviceInputFormResult = useDeviceInputForm({
    deviceId,
    initial:
      initial && !isCanvasEdit
        ? {
            inputFormat: initial.inputFormat,
            width: initial.width,
            height: initial.height,
            framerate: initial.framerate,
          }
        : undefined,
    autoSelect: mode === 'create',
  });

  // Default ffmpeg options on create
  useEffect(() => {
    if (mode === 'edit') return;
    api
      .GET('/api/options')
      .then((res) => {
        const data = unwrap(res, 'Failed to load options');
        setOptions((data.options ?? []).filter((o) => o.app_default).map((o) => o.key));
      })
      .catch(() => setOptions(['thread_queue_1024', 'copyts']));
  }, [mode]);

  // Standalone (non-canvas) streams eligible as canvas sources, excluding
  // the stream being edited itself. Includes currently-selected sources so
  // the form can render them in edit mode; the "+ Add source" dropdown
  // filters this list further to drop already-selected ids.
  const allSources = useMemo(
    () =>
      Object.values(streamsById).filter(
        (s): s is StreamData =>
          !!s && !s.canvas && s.stream_id !== initialData?.stream_id,
      ),
    [streamsById, initialData?.stream_id],
  );
  const availableSources = useMemo(
    () => allSources.filter((s) => !sourceIds.includes(s.stream_id)),
    [allSources, sourceIds],
  );

  const selectDevice = useCallback((id: string) => setDeviceId(id), []);

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

  const addAudioTrack = useCallback(() => {
    setAudioTracks((cur) => (cur.length >= 4 ? cur : [...cur, '']));
  }, []);

  const removeAudioTrack = useCallback((index: number) => {
    setAudioTracks((cur) => dropAt(cur, index));
  }, []);

  const updateAudioTrack = useCallback((index: number, value: string) => {
    setAudioTracks((cur) => {
      if (index < 0 || index >= cur.length) return cur;
      const next = [...cur];
      next[index] = value;
      return next;
    });
  }, []);

  const errors = useMemo(
    () =>
      computeErrors({
        mode,
        streamId,
        isMulti,
        deviceId,
        inputFormat: inputForm.inputFormat,
        sourceIds,
        fps,
        codec,
        bitrate,
      }),
    [mode, streamId, isMulti, deviceId, inputForm.inputFormat, sourceIds, fps, codec, bitrate],
  );

  const isValid = Object.keys(errors).length === 0;

  const isDirty = useMemo(() => {
    if (mode === 'create') return true;
    if (!initial) return false;
    const a = JSON.stringify;
    return (
      (!isMulti && inputForm.isDirty) ||
      codec !== initial.codec ||
      bitrate !== initial.bitrate ||
      a(audioTracks) !== a(initial.audioTracks) ||
      a(options) !== a(initial.options) ||
      rotation !== initial.rotation ||
      a(sourceIds) !== a(initial.sourceIds) ||
      a(rotationOverrides) !== a(initial.rotationOverrides) ||
      preset !== initial.preset ||
      fps !== initial.fps ||
      keyColor !== initial.keyColor ||
      layoutName !== initial.layoutName
    );
  }, [
    mode,
    initial,
    isMulti,
    inputForm.isDirty,
    codec,
    bitrate,
    audioTracks,
    options,
    rotation,
    sourceIds,
    preset,
    fps,
    keyColor,
    layoutName,
    rotationOverrides,
  ]);

  // Debounced layout preview for multi mode
  const [layout, setLayout] = useState<CanvasLayoutData | null>(null);
  const [layoutLoading, setLayoutLoading] = useState(false);
  const layoutTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const layoutSeqRef = useRef(0);
  useEffect(() => {
    if (!isMulti) {
      setLayout(null);
      setLayoutLoading(false);
      if (layoutTimerRef.current) clearTimeout(layoutTimerRef.current);
      return;
    }
    if (layoutTimerRef.current) clearTimeout(layoutTimerRef.current);
    const { width, height } = CANVAS_PRESETS[preset];
    const overrides: CanvasSourceOverrideData[] = rotationOverrides.map((r) =>
      r !== null ? { rotation: r } : {},
    );
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
          setLayout(apiError || !data ? null : data);
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
  }, [isMulti, sourceIds, rotationOverrides, preset, layoutName]);

  // Reset layout name if source count changes (n-specific named candidates).
  const lastCountRef = useRef(sourceIds.length);
  useEffect(() => {
    if (lastCountRef.current !== sourceIds.length) {
      lastCountRef.current = sourceIds.length;
      setLayoutName('');
    }
  }, [sourceIds.length]);

  const cycleLayout = useCallback(() => {
    const available = layout?.available_layouts ?? [];
    if (available.length <= 1) return;
    const current = layoutName || layout?.chosen_layout || available[0]!;
    const idx = available.indexOf(current);
    const next = available[(idx + 1) % available.length] ?? available[0]!;
    setLayoutName(next);
  }, [layout, layoutName]);

  const buildCanvas = useCallback((): CanvasData => {
    const { width, height } = CANVAS_PRESETS[preset];
    const overrides: CanvasSourceOverrideData[] = rotationOverrides.map((r) =>
      r !== null ? { rotation: r } : {},
    );
    const hasAnyOverride = rotationOverrides.some((r) => r !== null);
    return {
      width,
      height,
      fps,
      key_color: keyColor,
      source_streams: sourceIds,
      audio_devices: audioTracks.filter((d) => d.trim() !== ''),
      ...(hasAnyOverride ? { source_overrides: overrides } : {}),
      ...(layoutName ? { layout_name: layoutName } : {}),
    };
  }, [preset, fps, keyColor, sourceIds, audioTracks, rotationOverrides, layoutName]);

  const buildCreateRequest = useCallback((): StreamRequestData => {
    if (isMulti) {
      return {
        stream_id: streamId,
        codec,
        bitrate,
        canvas: buildCanvas(),
        ...(options.length > 0 ? { options } : {}),
      } as StreamRequestData;
    }
    const req: StreamRequestData = {
      stream_id: streamId,
      device_id: deviceId,
      codec,
      input_format: inputForm.inputFormat,
      ...(inputForm.width > 0 ? { width: inputForm.width } : {}),
      ...(inputForm.height > 0 ? { height: inputForm.height } : {}),
      ...(inputForm.framerate > 0 ? { framerate: inputForm.framerate } : {}),
      ...(bitrate ? { bitrate } : {}),
      ...(audioTracks[0] ? { audio_device: audioTracks[0] } : {}),
      ...(options.length > 0 ? { options } : {}),
    };
    if (rotation === 90 || rotation === 180 || rotation === 270) {
      req.rotation = rotation;
    }
    return req;
  }, [
    isMulti,
    streamId,
    deviceId,
    codec,
    inputForm.inputFormat,
    inputForm.width,
    inputForm.height,
    inputForm.framerate,
    bitrate,
    audioTracks,
    options,
    rotation,
    buildCanvas,
  ]);

  const buildEditPayload = useCallback((): Partial<StreamRequestData> => {
    if (!initial) return {};
    if (isMulti) {
      return diffMultiEdit(buildCanvas(), initial, codec, bitrate, options);
    }
    return diffSingleEdit(inputForm.diff(), initial, codec, bitrate, audioTracks, options, rotation);
  }, [isMulti, initial, inputForm, codec, bitrate, audioTracks, options, rotation, buildCanvas]);

  const submit = useCallback(async () => {
    if (!isValid) return false;
    setSaving(true);
    setError(null);
    try {
      if (mode === 'create') {
        await createStream(buildCreateRequest());
      } else if (initial) {
        const payload = buildEditPayload();
        if (Object.keys(payload).length > 0) {
          await updateStream(streamId, payload);
        }
      }
      return true;
    } catch (error_) {
      setError(error_ instanceof Error ? error_.message : 'Failed to save stream');
      return false;
    } finally {
      setSaving(false);
    }
  }, [
    isValid,
    mode,
    streamId,
    initial,
    buildCreateRequest,
    buildEditPayload,
    createStream,
    updateStream,
  ]);

  return {
    mode,
    // identity
    streamId,
    setStreamId,
    // single-source device + capabilities
    deviceId,
    selectDevice,
    inputForm,
    rotation,
    setRotation,
    // multi-source (canvas) controls
    isMulti,
    sourceIds,
    rotationOverrides,
    setRotationOverride,
    addSource,
    removeSource,
    moveSource,
    allSources,
    availableSources,
    preset,
    setPreset,
    fps,
    setFps,
    fpsOptions: FPS_OPTIONS,
    keyColor,
    setKeyColor,
    layoutName,
    setLayoutName,
    layout,
    layoutLoading,
    chosenLayout: layout?.chosen_layout ?? '',
    availableLayouts: layout?.available_layouts ?? [],
    cycleLayout,
    canvasDimensions: CANVAS_PRESETS[preset],
    // audio tracks (each entry → one output track)
    audioTracks,
    addAudioTrack,
    removeAudioTrack,
    updateAudioTrack,
    // stream-level encoder
    codec,
    setCodec,
    bitrate,
    setBitrate,
    options,
    setOptions,
    // form state
    errors,
    isValid,
    isDirty,
    saving,
    error,
    submit,
  };
}
