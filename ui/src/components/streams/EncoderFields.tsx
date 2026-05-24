import { InputField } from '../InputField';
import { Select } from '../Select';
import type { EncoderConfig, EncoderCodec, RateControl } from './types';

interface EncoderFieldsProps {
  value: EncoderConfig;
  customEncoderArgs: string;
  onChange: (next: EncoderConfig) => void;
  onCustomEncoderArgsChange: (next: string) => void;
  disabled?: boolean;
  errors: Record<string, string>;
}

const CODECS: { value: EncoderCodec; label: string }[] = [
  { value: 'h264', label: 'H.264' },
  { value: 'h265', label: 'H.265' },
  { value: 'av1', label: 'AV1' },
];

const RATE_CONTROLS: { value: RateControl; label: string }[] = [
  { value: 'cbr', label: 'CBR (constant)' },
  { value: 'vbr', label: 'VBR (variable)' },
  { value: 'cqp', label: 'CQP (constant quality)' },
];

const PRESETS = ['ultrafast', 'superfast', 'veryfast', 'faster', 'fast', 'medium', 'slow', 'slower', 'veryslow'];

export function EncoderFields({
  value,
  customEncoderArgs,
  onChange,
  onCustomEncoderArgsChange,
  disabled,
  errors,
}: Readonly<EncoderFieldsProps>) {
  const update = <K extends keyof EncoderConfig>(key: K, next: EncoderConfig[K]) =>
    onChange({ ...value, [key]: next });

  const codecErr = errors['encoder.codec'] ? { error: errors['encoder.codec'] } : {};
  const bitrateErr = errors['encoder.bitrate'] ? { error: errors['encoder.bitrate'] } : {};
  const gopErr = errors['encoder.gop'] ? { error: errors['encoder.gop'] } : {};

  return (
    <section className="space-y-4">
      <h2 className="text-lg font-semibold text-fg">Encoder</h2>
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <Select
          label="Codec"
          required
          value={value.codec}
          onChange={(e) => update('codec', e.target.value as EncoderCodec)}
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
          value={value.bitrate}
          onChange={(e) => update('bitrate', e.target.value)}
          placeholder="2M"
          required
          disabled={disabled}
          hint="e.g. 2M, 6000k"
          {...bitrateErr}
        />
        <InputField
          label="GOP (keyframe interval)"
          type="number"
          min={0}
          value={value.gop ?? ''}
          onChange={(e) => {
            const n = e.target.value === '' ? undefined : parseInt(e.target.value, 10);
            update('gop', Number.isNaN(n) ? undefined : n);
          }}
          placeholder="120"
          disabled={disabled}
          {...gopErr}
        />
        <Select
          label="Rate Control"
          value={value.rate_control ?? ''}
          onChange={(e) => update('rate_control', (e.target.value || undefined) as RateControl | undefined)}
          disabled={disabled}
        >
          <option value="">(encoder default)</option>
          {RATE_CONTROLS.map((rc) => (
            <option key={rc.value} value={rc.value}>
              {rc.label}
            </option>
          ))}
        </Select>
        <Select
          label="Preset"
          value={value.preset ?? ''}
          onChange={(e) => update('preset', e.target.value || undefined)}
          disabled={disabled}
        >
          <option value="">(encoder default)</option>
          {PRESETS.map((p) => (
            <option key={p} value={p}>
              {p}
            </option>
          ))}
        </Select>
      </div>
      <div>
        <label className="block text-sm font-medium text-fg mb-1" htmlFor="custom-encoder-args">
          Custom encoder args
        </label>
        <textarea
          id="custom-encoder-args"
          value={customEncoderArgs}
          onChange={(e) => onCustomEncoderArgsChange(e.target.value)}
          rows={3}
          disabled={disabled}
          placeholder="-x264-params keyint=120:bframes=0"
          className="block w-full rounded-sm border border-border-strong px-3 py-2 text-sm font-mono bg-surface text-fg placeholder:text-fg-subtle focus:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:border-accent disabled:opacity-50"
        />
        <p className="text-xs text-fg-muted mt-1">
          Escape hatch — passed verbatim to ffmpeg. Overrides codec/bitrate when set.
        </p>
      </div>
    </section>
  );
}
