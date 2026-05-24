import { useCallback, useMemo, useState } from 'react';
import { useStreamStore } from './useStreamStore';
import type { components } from '../lib/api.generated';
import type {
  AudioConfig,
  EncoderConfig,
  PublishTarget,
  StreamFormValue,
} from '../components/streams/types';

type StreamData = components['schemas']['StreamData'];
type StreamRequestData = components['schemas']['StreamRequestData'];

const STREAM_ID_RE = /^[\da-z][\da-z-]*[\da-z]$|^[\da-z]$/i;
const UPSTREAM_RE = /^(source|composer):[\da-z][\da-z-]*$/i;

function defaultEncoder(): EncoderConfig {
  return { codec: 'h264', bitrate: '2M' };
}

function defaultAudio(): AudioConfig {
  return { codec: 'aac', devices: [] };
}

function emptyValue(): StreamFormValue {
  return {
    stream_id: '',
    upstream: '',
    encoder: defaultEncoder(),
    audio: defaultAudio(),
    publish: [],
  };
}

function fromStreamData(s: StreamData | undefined): StreamFormValue {
  if (!s) return emptyValue();
  const encoder: EncoderConfig = { ...defaultEncoder(), ...(s.encoder as Partial<EncoderConfig>) };
  const audio: AudioConfig = { ...defaultAudio(), ...(s.audio as Partial<AudioConfig>) };
  return {
    stream_id: s.stream_id,
    upstream: s.upstream ?? '',
    encoder,
    audio,
    publish: (s.publish as PublishTarget[] | undefined) ?? [],
    custom_encoder_args: s.custom_encoder_args ?? '',
  };
}

interface ValidateOpts {
  mode: 'create' | 'edit';
  value: StreamFormValue;
  existingIds: string[];
}

function validate({ mode, value, existingIds }: ValidateOpts): Record<string, string> {
  const errors: Record<string, string> = {};

  const id = value.stream_id.trim();
  if (mode === 'create') {
    if (!id) {
      errors.stream_id = 'Stream ID is required';
    } else if (!STREAM_ID_RE.test(id)) {
      errors.stream_id = 'Stream ID must be alphanumeric with dashes';
    } else if (existingIds.includes(id)) {
      errors.stream_id = 'Stream ID already in use';
    }
  }

  const up = value.upstream.trim();
  if (!up) {
    errors.upstream = 'Upstream is required';
  } else if (!UPSTREAM_RE.test(up)) {
    errors.upstream = 'Upstream must be "source:<id>" or "composer:<id>"';
  }

  if (!value.encoder.codec) {
    errors['encoder.codec'] = 'Codec is required';
  }
  if (!value.encoder.bitrate.trim()) {
    errors['encoder.bitrate'] = 'Bitrate is required';
  }
  if (value.encoder.gop !== undefined && value.encoder.gop < 0) {
    errors['encoder.gop'] = 'GOP must be non-negative';
  }

  for (const [i, target] of value.publish.entries()) {
    if (!target.url.trim()) {
      errors[`publish.${i}.url`] = 'URL is required';
    }
  }

  return errors;
}

export interface UseStreamFormResult {
  mode: 'create' | 'edit';
  value: StreamFormValue;
  setStreamId: (next: string) => void;
  setUpstream: (next: string) => void;
  setEncoder: (next: EncoderConfig) => void;
  setAudio: (next: AudioConfig) => void;
  setPublish: (next: PublishTarget[]) => void;
  setCustomEncoderArgs: (next: string) => void;
  errors: Record<string, string>;
  isValid: boolean;
  isDirty: boolean;
  saving: boolean;
  error: string | null;
  submit: () => Promise<boolean>;
}

export function useStreamForm(initialData?: StreamData): UseStreamFormResult {
  const mode: 'create' | 'edit' = initialData ? 'edit' : 'create';

  const initialValue = useMemo(() => fromStreamData(initialData), [initialData]);
  const [value, setValue] = useState<StreamFormValue>(initialValue);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const createStream = useStreamStore((s) => s.createStream);
  const updateStream = useStreamStore((s) => s.updateStream);
  const streamIds = useStreamStore((s) => s.streamIds);

  const existingIds = useMemo(() => {
    if (mode === 'create') return streamIds;
    return streamIds.filter((id) => id !== initialData?.stream_id);
  }, [mode, streamIds, initialData?.stream_id]);

  const setStreamId = useCallback(
    (next: string) => setValue((cur) => ({ ...cur, stream_id: next })),
    [],
  );
  const setUpstream = useCallback(
    (next: string) => setValue((cur) => ({ ...cur, upstream: next })),
    [],
  );
  const setEncoder = useCallback(
    (next: EncoderConfig) => setValue((cur) => ({ ...cur, encoder: next })),
    [],
  );
  const setAudio = useCallback(
    (next: AudioConfig) => setValue((cur) => ({ ...cur, audio: next })),
    [],
  );
  const setPublish = useCallback(
    (next: PublishTarget[]) => setValue((cur) => ({ ...cur, publish: next })),
    [],
  );
  const setCustomEncoderArgs = useCallback(
    (next: string) => setValue((cur) => ({ ...cur, custom_encoder_args: next })),
    [],
  );

  const errors = useMemo(
    () => validate({ mode, value, existingIds }),
    [mode, value, existingIds],
  );
  const isValid = Object.keys(errors).length === 0;

  const isDirty = useMemo(() => {
    if (mode === 'create') return true;
    return JSON.stringify(value) !== JSON.stringify(initialValue);
  }, [mode, value, initialValue]);

  const submit = useCallback(async () => {
    if (!isValid) return false;
    setSaving(true);
    setError(null);
    try {
      // Cast through the legacy StreamRequestData shape until the API
      // regen lands the new payload. The new fields (upstream, encoder,
      // audio, publish, custom_encoder_args) flow through the body
      // verbatim because the store passes the body to fetch without
      // schema-enforced stripping.
      const body = {
        stream_id: value.stream_id,
        upstream: value.upstream,
        encoder: value.encoder,
        audio: value.audio,
        publish: value.publish,
        ...(value.custom_encoder_args ? { custom_encoder_args: value.custom_encoder_args } : {}),
      } as unknown as StreamRequestData;

      if (mode === 'create') {
        await createStream(body);
      } else if (initialData) {
        await updateStream(initialData.stream_id, body as Partial<StreamRequestData>);
      }
      return true;
    } catch (error_) {
      setError(error_ instanceof Error ? error_.message : 'Failed to save stream');
      return false;
    } finally {
      setSaving(false);
    }
  }, [isValid, mode, value, initialData, createStream, updateStream]);

  return {
    mode,
    value,
    setStreamId,
    setUpstream,
    setEncoder,
    setAudio,
    setPublish,
    setCustomEncoderArgs,
    errors,
    isValid,
    isDirty,
    saving,
    error,
    submit,
  };
}
