import { useEffect } from 'react';
import { Spinner } from '../Spinner';
import {
  useDeviceFormats,
  useDeviceResolutions,
  useDeviceFramerates,
} from '../../hooks/useDeviceCapabilities';
import type { components } from '../../lib/api.generated';

type FormatName = components['schemas']['SourceFormatBody']['format_name'];

export interface SourceFormatValue {
  format_name: FormatName | '';
  width: number;
  height: number;
  fps: number;
}

interface SourceFormatSelectorProps {
  deviceId: string;
  value: SourceFormatValue;
  onChange: (next: SourceFormatValue) => void;
  disabled?: boolean;
}

const selectClasses =
  'block w-full px-3 py-2 border border-border rounded-md shadow-sm bg-surface text-fg focus:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:border-accent disabled:opacity-60';

const EMPTY_VALUE: SourceFormatValue = { format_name: '', width: 0, height: 0, fps: 0 };

export function SourceFormatSelector({
  deviceId,
  value,
  onChange,
  disabled = false,
}: Readonly<SourceFormatSelectorProps>) {
  const { formats, loading: loadingFormats, error: formatsError } = useDeviceFormats(deviceId);

  const formatName: FormatName | undefined = value.format_name || undefined;
  const { resolutions, loading: loadingResolutions } = useDeviceResolutions(deviceId, formatName);
  const { framerates, loading: loadingFramerates } = useDeviceFramerates(
    deviceId,
    formatName,
    value.width || undefined,
    value.height || undefined,
  );

  // Cascade auto-pick: when the current selection isn't in the freshly
  // loaded list, default to the first available so the user doesn't have
  // to clear an invalid choice manually.
  useEffect(() => {
    if (loadingFormats || formats.length === 0) return;
    if (value.format_name && formats.some((f) => f.format_name === value.format_name)) return;
    const first = formats[0]?.format_name;
    if (first) onChange({ ...EMPTY_VALUE, format_name: first });
  }, [loadingFormats, formats, value.format_name, onChange]);

  useEffect(() => {
    if (loadingResolutions || !formatName) return;
    if (resolutions.length === 0) return;
    const match = resolutions.find((r) => r.width === value.width && r.height === value.height);
    if (match) return;
    const first = resolutions[0];
    if (!first) return;
    onChange({ format_name: formatName, width: first.width, height: first.height, fps: 0 });
  }, [loadingResolutions, formatName, resolutions, value.width, value.height, onChange]);

  useEffect(() => {
    if (loadingFramerates || !formatName || !value.width || !value.height) return;
    if (framerates.length === 0) return;
    const match = framerates.find((f) => Math.round(f.fps) === value.fps);
    if (match) return;
    const first = framerates[0];
    if (!first) return;
    onChange({
      format_name: formatName,
      width: value.width,
      height: value.height,
      fps: Math.round(first.fps),
    });
  }, [loadingFramerates, formatName, framerates, value.width, value.height, value.fps, onChange]);

  if (formatsError) {
    return (
      <div className="p-3 border border-danger rounded-md bg-danger-soft">
        <p className="text-sm text-danger-soft-fg">Failed to load formats: {formatsError}</p>
      </div>
    );
  }

  if (loadingFormats && formats.length === 0) {
    return (
      <div className="flex items-center space-x-2 text-sm">
        <Spinner size="xs" />
        <span className="text-fg-muted">Loading capture formats…</span>
      </div>
    );
  }

  if (formats.length === 0) {
    return (
      <div className="p-3 border border-warning rounded-md bg-warning-soft">
        <p className="text-sm text-warning-soft-fg">
          No supported formats reported by this device.
        </p>
      </div>
    );
  }

  const resolutionKey = value.width && value.height ? `${value.width}x${value.height}` : '';

  return (
    <div className="grid gap-3 sm:grid-cols-3">
      <div>
        <label htmlFor="src-fmt" className="block text-sm font-medium text-fg mb-1">
          Format
        </label>
        <select
          id="src-fmt"
          aria-label="Input format"
          className={selectClasses}
          disabled={disabled}
          value={value.format_name}
          onChange={(e) => {
            const next = e.target.value as FormatName | '';
            onChange({ format_name: next, width: 0, height: 0, fps: 0 });
          }}
        >
          {formats.map((f) => (
            <option key={f.format_name} value={f.format_name}>
              {f.format_name.toUpperCase()}
              {f.emulated ? ' (emulated)' : ''}
            </option>
          ))}
        </select>
      </div>

      <div>
        <label htmlFor="src-res" className="block text-sm font-medium text-fg mb-1">
          Resolution
        </label>
        <select
          id="src-res"
          aria-label="Resolution"
          className={selectClasses}
          disabled={disabled || loadingResolutions || resolutions.length === 0}
          value={resolutionKey}
          onChange={(e) => {
            const [w, h] = e.target.value.split('x').map(Number);
            if (!w || !h || !formatName) return;
            onChange({ format_name: formatName, width: w, height: h, fps: 0 });
          }}
        >
          {resolutions.length === 0 && <option value="">—</option>}
          {resolutions.map((r) => (
            <option key={`${r.width}x${r.height}`} value={`${r.width}x${r.height}`}>
              {r.width} × {r.height}
            </option>
          ))}
        </select>
      </div>

      <div>
        <label htmlFor="src-fps" className="block text-sm font-medium text-fg mb-1">
          Framerate
        </label>
        <select
          id="src-fps"
          aria-label="Framerate"
          className={selectClasses}
          disabled={disabled || loadingFramerates || framerates.length === 0}
          value={value.fps || ''}
          onChange={(e) => {
            const fps = Number(e.target.value);
            if (!formatName) return;
            onChange({
              format_name: formatName,
              width: value.width,
              height: value.height,
              fps: Number.isFinite(fps) ? fps : 0,
            });
          }}
        >
          <option value="">Driver default</option>
          {framerates.map((f) => {
            const rounded = Math.round(f.fps);
            return (
              <option key={`${f.numerator}/${f.denominator}`} value={rounded}>
                {rounded} fps
              </option>
            );
          })}
        </select>
      </div>
    </div>
  );
}
