import { useCallback, useEffect, useMemo, useState } from 'react';
import type { components } from '../lib/api.generated';
import {
  useDeviceFormats,
  useDeviceResolutions,
  useDeviceFramerates,
} from './useDeviceCapabilities';
import { RESOLUTION_LABELS } from '../components/StreamCreation/constants';

type FormatInfo = components['schemas']['FormatInfo'];
type FormatName = FormatInfo['format_name'];
type Resolution = components['schemas']['Resolution'];

export interface DeviceInputSelectOption {
  value: string;
  label: string;
}

export interface DeviceInputInitial {
  inputFormat?: string;
  width?: number;
  height?: number;
  framerate?: number;
}

export interface UseDeviceInputFormOptions {
  deviceId: string;
  initial?: DeviceInputInitial | undefined;
  // Run cascading auto-select when capabilities load. Default true (create
  // flow); edit/sheet flows pass false to keep the user's existing values
  // unless they explicitly change them.
  autoSelect?: boolean | undefined;
}

export interface DeviceInputDiff {
  input_format?: string;
  width?: number;
  height?: number;
  framerate?: number;
}

export interface UseDeviceInputFormResult {
  inputFormat: string;
  width: number;
  height: number;
  framerate: number;

  formatOptions: DeviceInputSelectOption[];
  resolutionOptions: DeviceInputSelectOption[];
  framerateOptions: DeviceInputSelectOption[];

  formatsLoading: boolean;
  resolutionsLoading: boolean;
  frameratesLoading: boolean;

  selectFormat: (fmt: string) => void;
  selectResolution: (w: number, h: number) => void;
  setFramerate: (fps: number) => void;

  isDirty: boolean;
  diff: () => DeviceInputDiff;
}

const MANUAL_FPS_OPTIONS: DeviceInputSelectOption[] = [
  { value: '24', label: '24 FPS' },
  { value: '25', label: '25 FPS' },
  { value: '30', label: '30 FPS' },
  { value: '50', label: '50 FPS' },
  { value: '60', label: '60 FPS' },
];

function filterToCommonResolutions(resolutions: Resolution[]): Resolution[] {
  const common = resolutions.filter(
    (res) => `${res.width}x${res.height}` in RESOLUTION_LABELS,
  );
  return common.length > 0 ? common : resolutions;
}

type AutoPhase = 'idle' | 'selecting-format' | 'selecting-resolution';
const PHASE_FORMAT: AutoPhase = 'selecting-format';
const PHASE_RESOLUTION: AutoPhase = 'selecting-resolution';

export function useDeviceInputForm(
  options: UseDeviceInputFormOptions,
): UseDeviceInputFormResult {
  const { deviceId, initial, autoSelect = true } = options;

  const [inputFormat, setInputFormat] = useState<FormatName | ''>(
    (initial?.inputFormat ?? '') as FormatName | '',
  );
  const [width, setWidth] = useState<number>(initial?.width ?? 0);
  const [height, setHeight] = useState<number>(initial?.height ?? 0);
  const [framerate, setFramerateState] = useState<number>(initial?.framerate ?? 0);
  const [autoSelectPhase, setAutoSelectPhase] = useState<AutoPhase>('idle');

  const { formats, loading: formatsLoading } = useDeviceFormats(deviceId);
  const { resolutions: rawResolutions, loading: resolutionsLoading } = useDeviceResolutions(
    deviceId,
    inputFormat || undefined,
  );
  const { framerates: apiFramerates, loading: frameratesLoading } = useDeviceFramerates(
    deviceId,
    inputFormat || undefined,
    width,
    height,
  );

  const resolutions = useMemo(
    () => filterToCommonResolutions(rawResolutions),
    [rawResolutions],
  );

  const formatOptions = useMemo((): DeviceInputSelectOption[] => {
    const opts = formats.map((f) => ({
      value: f.format_name,
      label: `${f.format_name.toUpperCase()} - ${f.original_name}`,
    }));
    if (inputFormat && !opts.some((o) => o.value === inputFormat)) {
      opts.unshift({
        value: inputFormat,
        label: `${inputFormat.toUpperCase()}${autoSelect ? '' : ' (current)'}`,
      });
    }
    return opts;
  }, [formats, inputFormat, autoSelect]);

  const resolutionOptions = useMemo((): DeviceInputSelectOption[] => {
    const opts = resolutions.map((res) => {
      const resString = `${res.width}x${res.height}`;
      const friendlyName = RESOLUTION_LABELS[resString];
      return {
        value: resString,
        label: friendlyName ? `${resString} (${friendlyName})` : resString,
      };
    });
    if (width > 0 && height > 0) {
      const currentVal = `${width}x${height}`;
      if (!opts.some((o) => o.value === currentVal)) {
        const friendlyName = RESOLUTION_LABELS[currentVal];
        const suffix = autoSelect ? '' : ' (current)';
        const label = friendlyName
          ? `${currentVal} (${friendlyName})${suffix}`
          : `${currentVal}${suffix}`;
        opts.unshift({ value: currentVal, label });
      }
    }
    return opts;
  }, [resolutions, width, height, autoSelect]);

  const framerateOptions = useMemo((): DeviceInputSelectOption[] => {
    const currentSuffix = autoSelect ? '' : ' (current)';
    const addCurrentFallback = (opts: DeviceInputSelectOption[]) => {
      if (framerate > 0 && !opts.some((o) => o.value === framerate.toString())) {
        opts.unshift({
          value: framerate.toString(),
          label: `${framerate} FPS${currentSuffix}`,
        });
      }
    };

    if (apiFramerates.length > 0 && width > 0 && height > 0) {
      const opts = apiFramerates.map((rate) => {
        const fpsValue = Math.round(rate.fps);
        return {
          value: fpsValue.toString(),
          label: `${fpsValue} FPS (${rate.numerator}/${rate.denominator})`,
        };
      });
      addCurrentFallback(opts);
      return opts;
    }
    const opts = MANUAL_FPS_OPTIONS.map((o) => ({ ...o }));
    addCurrentFallback(opts);
    return opts;
  }, [apiFramerates, width, height, framerate, autoSelect]);

  // Auto-select preferred format when capabilities first load.
  useEffect(() => {
    if (!autoSelect) return;
    if (formats.length > 0 && deviceId && !inputFormat) {
      const preferred = formats.find((f) => !f.emulated) ?? formats[0];
      if (preferred) {
        setInputFormat(preferred.format_name);
        setAutoSelectPhase(PHASE_FORMAT);
      }
    }
  }, [formats, deviceId, inputFormat, autoSelect]);

  // Auto-select highest resolution after format is chosen.
  useEffect(() => {
    if (!autoSelect) return;
    if (
      resolutions.length > 0 &&
      inputFormat &&
      width === 0 &&
      height === 0 &&
      autoSelectPhase === PHASE_FORMAT
    ) {
      const highest = [...resolutions].sort(
        (a, b) => b.width * b.height - a.width * a.height,
      )[0];
      if (highest) {
        setWidth(highest.width);
        setHeight(highest.height);
        setFramerateState(0);
        setAutoSelectPhase(PHASE_RESOLUTION);
      }
    }
  }, [resolutions, inputFormat, width, height, autoSelectPhase, autoSelect]);

  // Auto-select highest framerate after resolution is chosen.
  useEffect(() => {
    if (!autoSelect) return;
    if (apiFramerates.length === 0 || width <= 0 || height <= 0) return;
    if (autoSelectPhase !== PHASE_RESOLUTION) return;

    const currentIsValid =
      framerate > 0 && apiFramerates.some((fr) => Math.round(fr.fps) === framerate);
    if (!currentIsValid) {
      const highest = [...apiFramerates].sort((a, b) => b.fps - a.fps)[0];
      if (highest) setFramerateState(Math.round(highest.fps));
    }
    setAutoSelectPhase('idle');
  }, [apiFramerates, width, height, framerate, autoSelectPhase, autoSelect]);

  // Reset state when the device changes — cascade down from format.
  useEffect(() => {
    setInputFormat((initial?.inputFormat ?? '') as FormatName | '');
    setWidth(initial?.width ?? 0);
    setHeight(initial?.height ?? 0);
    setFramerateState(initial?.framerate ?? 0);
    setAutoSelectPhase('idle');
    // Intentionally only re-run on deviceId change.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [deviceId]);

  const selectFormat = useCallback((fmt: string) => {
    setInputFormat(fmt as FormatName | '');
    setWidth(0);
    setHeight(0);
    setFramerateState(0);
    setAutoSelectPhase(PHASE_FORMAT);
  }, []);

  const selectResolution = useCallback((w: number, h: number) => {
    setWidth(w);
    setHeight(h);
    setFramerateState(0);
    setAutoSelectPhase(PHASE_RESOLUTION);
  }, []);

  const setFramerate = useCallback((fps: number) => {
    setFramerateState(fps);
  }, []);

  const isDirty = useMemo(() => {
    if (!initial) return inputFormat !== '' || width !== 0 || height !== 0 || framerate !== 0;
    return (
      inputFormat !== (initial.inputFormat ?? '') ||
      width !== (initial.width ?? 0) ||
      height !== (initial.height ?? 0) ||
      framerate !== (initial.framerate ?? 0)
    );
  }, [initial, inputFormat, width, height, framerate]);

  const diff = useCallback((): DeviceInputDiff => {
    const out: DeviceInputDiff = {};
    if (inputFormat !== (initial?.inputFormat ?? '')) out.input_format = inputFormat;
    if (width !== (initial?.width ?? 0)) out.width = width;
    if (height !== (initial?.height ?? 0)) out.height = height;
    if (framerate !== (initial?.framerate ?? 0)) out.framerate = framerate;
    return out;
  }, [initial, inputFormat, width, height, framerate]);

  return {
    inputFormat,
    width,
    height,
    framerate,
    formatOptions,
    resolutionOptions,
    framerateOptions,
    formatsLoading,
    resolutionsLoading,
    frameratesLoading,
    selectFormat,
    selectResolution,
    setFramerate,
    isDirty,
    diff,
  };
}
