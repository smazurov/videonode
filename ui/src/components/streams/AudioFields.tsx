import { useMemo } from 'react';
import { InputField } from '../InputField';
import { Select } from '../Select';
import { MultiSelect, type MultiSelectOption } from '../MultiSelect';
import { useAudioDevices } from '../../hooks/useAudioDevices';
import type { AudioCodec, AudioConfig } from './types';

interface AudioFieldsProps {
  value: AudioConfig;
  onChange: (next: AudioConfig) => void;
  disabled?: boolean;
  errors: Record<string, string>;
}

const CODECS: { value: AudioCodec; label: string }[] = [
  { value: 'aac', label: 'AAC' },
  { value: 'opus', label: 'Opus' },
];

export function AudioFields({
  value,
  onChange,
  disabled,
  errors,
}: Readonly<AudioFieldsProps>) {
  const { devices, loading } = useAudioDevices();

  const options = useMemo<MultiSelectOption[]>(() => {
    const fromAPI: MultiSelectOption[] = devices.map((d) => ({
      value: d.alsa_device,
      label: `${d.card_name} (${d.alsa_device})`,
    }));
    // Preserve any device that was saved but is currently unplugged so
    // the user can still see/deselect it.
    const known = new Set(fromAPI.map((o) => o.value));
    for (const dev of value.devices ?? []) {
      if (!known.has(dev)) {
        fromAPI.push({ value: dev, label: `${dev} (offline)` });
      }
    }
    return fromAPI;
  }, [devices, value.devices]);

  const update = <K extends keyof AudioConfig>(key: K, next: AudioConfig[K]) =>
    onChange({ ...value, [key]: next });

  const codecErr = errors['audio.codec'] ? { error: errors['audio.codec'] } : {};
  const bitrateErr = errors['audio.bitrate'] ? { error: errors['audio.bitrate'] } : {};

  return (
    <section className="space-y-4">
      <h2 className="text-lg font-semibold text-fg">Audio</h2>
      <div className="grid grid-cols-1 md:grid-cols-4 gap-4 items-start">
        <div className="space-y-1 md:col-span-2">
          <label className="block text-sm font-medium text-fg">ALSA Devices</label>
          <MultiSelect
            options={options}
            selected={value.devices ?? []}
            onChange={(next) => update('devices', next)}
            placeholder={loading ? 'Loading devices...' : 'Select audio inputs'}
            label="ALSA devices"
            size="md"
          />
          <p className="text-xs text-fg-muted">
            Pick one or more capture devices. Multiple devices are mixed via the filter below.
          </p>
        </div>
        <Select
          label="Codec"
          value={value.codec}
          onChange={(e) => update('codec', e.target.value as AudioCodec)}
          disabled={disabled}
          {...codecErr}
        >
          {CODECS.map((c) => (
            <option key={c.value} value={c.value}>
              {c.label}
            </option>
          ))}
        </Select>
        <InputField
          label="Bitrate"
          type="text"
          value={value.bitrate ?? ''}
          onChange={(e) => update('bitrate', e.target.value || undefined)}
          placeholder="192k"
          disabled={disabled}
          hint="e.g. 128k, 192k"
          {...bitrateErr}
        />
      </div>
      <InputField
        label="Mix filter (optional)"
        type="text"
        value={value.filters ?? ''}
        onChange={(e) => update('filters', e.target.value || undefined)}
        placeholder="amix=inputs=2:duration=shortest"
        disabled={disabled}
        hint="Custom ffmpeg -filter_complex audio chain."
      />
    </section>
  );
}
