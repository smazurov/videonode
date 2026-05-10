import { useState, useMemo, useCallback, useEffect } from 'react';
import { useStreamStore } from './useStreamStore';
import { useDeviceInputForm, type UseDeviceInputFormResult } from './useDeviceInputForm';
import type { components } from '../lib/api.generated';
import { api, unwrap } from '../lib/api';
import { parseResolution } from '../lib/resolution';

type StreamData = components["schemas"]["StreamData"];
type StreamRequestData = components["schemas"]["StreamRequestData"];

interface ParsedStreamData {
  streamId: string;
  deviceId: string;
  inputFormat: string;
  width: number;
  height: number;
  framerate: number;
  codec: string;
  bitrate: number | undefined;
  audioDevice: string;
  options: string[];
  rotation: number;
}

function parseStreamData(sd: StreamData): ParsedStreamData {
  const { width, height } = parseResolution(sd.resolution);
  return {
    streamId: sd.stream_id,
    deviceId: sd.device_id,
    inputFormat: sd.input_format || '',
    width,
    height,
    framerate: sd.framerate ? parseInt(sd.framerate, 10) || 0 : 0,
    codec: sd.codec,
    bitrate: sd.bitrate ? parseFloat(sd.bitrate.replace(/[^\d.]/g, '')) : undefined,
    audioDevice: sd.audio_device || '',
    options: sd.options || [],
    rotation: sd.rotation ?? 0,
  };
}

export function useStreamForm(initialData?: StreamData) {
  const mode = initialData ? ('edit' as const) : ('create' as const);
  const initial = useMemo(() => (initialData ? parseStreamData(initialData) : null), [initialData]);

  const [streamId, setStreamId] = useState(initial?.streamId ?? '');
  const [deviceId, setDeviceId] = useState(initial?.deviceId ?? '');
  const [codec, setCodec] = useState(initial?.codec ?? 'h264');
  const [bitrate, setBitrate] = useState<number | undefined>(initial?.bitrate ?? 2);
  const [audioDevice, setAudioDevice] = useState(initial?.audioDevice ?? '');
  const [options, setOptions] = useState<string[]>(initial?.options ?? []);
  const [rotation, setRotation] = useState<number>(initial?.rotation ?? 0);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const createStream = useStreamStore((s) => s.createStream);
  const updateStream = useStreamStore((s) => s.updateStream);

  const inputForm: UseDeviceInputFormResult = useDeviceInputForm({
    deviceId,
    initial: initial
      ? {
          inputFormat: initial.inputFormat,
          width: initial.width,
          height: initial.height,
          framerate: initial.framerate,
        }
      : undefined,
    autoSelect: mode === 'create',
  });

  useEffect(() => {
    if (mode === 'edit') return;
    api.GET("/api/options")
      .then((res) => {
        const data = unwrap(res, 'Failed to load options');
        setOptions((data.options ?? []).filter((o) => o.app_default).map((o) => o.key));
      })
      .catch(() => setOptions(['thread_queue_1024', 'copyts']));
  }, [mode]);

  const selectDevice = useCallback((id: string) => {
    setDeviceId(id);
  }, []);

  const errors = useMemo(() => {
    const e: Record<string, string> = {};
    if (!streamId.trim()) {
      e.streamId = 'Stream ID is required';
    } else if (!/^[\w-]+$/.test(streamId)) {
      e.streamId = 'Stream ID can only contain letters, numbers, dashes, and underscores';
    }
    if (mode === 'create' && !deviceId) e.deviceId = 'Device selection is required';
    if (mode === 'create' && !inputForm.inputFormat) e.input_format = 'Format selection is required';
    if (!codec) e.codec = 'Codec selection is required';
    if (bitrate !== undefined && (bitrate < 0.1 || bitrate > 50))
      e.bitrate = 'Bitrate must be between 0.1 and 50 Mbps';
    return e;
  }, [streamId, deviceId, inputForm.inputFormat, codec, bitrate, mode]);

  const isValid = Object.keys(errors).length === 0;

  const isDirty = useMemo(() => {
    if (mode === 'create') return true;
    if (!initial) return false;
    return (
      inputForm.isDirty ||
      codec !== initial.codec ||
      bitrate !== initial.bitrate ||
      audioDevice !== initial.audioDevice ||
      JSON.stringify(options) !== JSON.stringify(initial.options) ||
      rotation !== initial.rotation
    );
  }, [mode, initial, inputForm.isDirty, codec, bitrate, audioDevice, options, rotation]);

  const buildCreateRequest = useCallback((): StreamRequestData => {
    const req: StreamRequestData = {
      stream_id: streamId,
      device_id: deviceId,
      codec: codec as StreamRequestData["codec"],
      input_format: inputForm.inputFormat,
      ...(inputForm.width > 0 ? { width: inputForm.width } : {}),
      ...(inputForm.height > 0 ? { height: inputForm.height } : {}),
      ...(inputForm.framerate > 0 ? { framerate: inputForm.framerate } : {}),
      ...(bitrate ? { bitrate } : {}),
      ...(audioDevice ? { audio_device: audioDevice } : {}),
      ...(options.length > 0 ? { options } : {}),
    };
    if (rotation === 90 || rotation === 180 || rotation === 270) {
      req.rotation = rotation;
    }
    return req;
  }, [streamId, deviceId, codec, inputForm.inputFormat, inputForm.width, inputForm.height, inputForm.framerate, bitrate, audioDevice, options, rotation]);

  const buildEditDiff = useCallback((): Record<string, string | number | string[] | undefined> => {
    if (!initial) return {};
    const changes: Record<string, string | number | string[] | undefined> = {
      ...inputForm.diff(),
    };
    if (codec !== initial.codec) changes.codec = codec;
    if (bitrate !== initial.bitrate) changes.bitrate = bitrate;
    if (audioDevice !== initial.audioDevice) changes.audio_device = audioDevice;
    if (JSON.stringify(options) !== JSON.stringify(initial.options)) changes.options = options;
    if (rotation !== initial.rotation) changes.rotation = rotation;
    return changes;
  }, [initial, inputForm, codec, bitrate, audioDevice, options, rotation]);

  const submit = useCallback(async () => {
    if (!isValid) return false;
    setSaving(true);
    setError(null);
    try {
      if (mode === 'create') {
        await createStream(buildCreateRequest());
      } else if (initial) {
        const changes = buildEditDiff();
        if (Object.keys(changes).length > 0) {
          await updateStream(streamId, changes);
        }
      }
      return true;
    } catch (error_) {
      setError(error_ instanceof Error ? error_.message : 'Failed to save stream');
      return false;
    } finally {
      setSaving(false);
    }
  }, [isValid, mode, streamId, initial, buildCreateRequest, buildEditDiff, createStream, updateStream]);

  return {
    mode,
    streamId,
    setStreamId,
    deviceId,
    selectDevice,
    inputForm,
    codec,
    setCodec,
    bitrate,
    setBitrate,
    audioDevice,
    setAudioDevice,
    options,
    setOptions,
    rotation,
    setRotation,
    errors,
    isValid,
    isDirty,
    saving,
    error,
    submit,
  };
}
