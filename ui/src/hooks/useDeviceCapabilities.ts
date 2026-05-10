import type { components } from '../lib/api.generated';
import { api, unwrap } from '../lib/api';
import { useAbortableQuery } from './useAbortableQuery';

type FormatInfo = components["schemas"]["FormatInfo"];
type FormatName = FormatInfo["format_name"];
type Resolution = components["schemas"]["Resolution"];
type Framerate = components["schemas"]["Framerate"];

export interface DeviceFrameratesResult {
  framerates: Framerate[];
  loading: boolean;
  error: string | null;
}

export function useDeviceFormats(deviceId: string) {
  const { data: formats, loading, error } = useAbortableQuery<FormatInfo[]>(
    async (signal) => {
      const result = unwrap(
        await api.GET("/api/devices/{device_id}/formats", {
          params: { path: { device_id: deviceId } },
          signal,
        }),
        'Failed to fetch formats',
      );
      return result.formats ?? [];
    },
    [deviceId],
    { initial: [], enabled: !!deviceId },
  );

  return { formats, loading, error };
}

export function useDeviceResolutions(deviceId: string, formatName: FormatName | undefined) {
  const enabled = !!deviceId && !!formatName;
  const { data: resolutions, loading, error } = useAbortableQuery<Resolution[]>(
    async (signal) => {
      const result = unwrap(
        await api.GET("/api/devices/{device_id}/resolutions", {
          params: {
            path: { device_id: deviceId },
            query: { format_name: formatName as FormatName },
          },
          signal,
        }),
        'Failed to fetch resolutions',
      );
      return result.resolutions ?? [];
    },
    [deviceId, formatName],
    { initial: [], enabled },
  );

  return { resolutions, loading, error };
}

export function useDeviceFramerates(
  deviceId: string | undefined,
  formatName: FormatName | undefined,
  width: number | undefined,
  height: number | undefined,
): DeviceFrameratesResult {
  const enabled = !!deviceId && !!formatName && !!width && !!height;
  const { data: framerates, loading, error } = useAbortableQuery<Framerate[]>(
    async (signal) => {
      const result = unwrap(
        await api.GET("/api/devices/{device_id}/framerates", {
          params: {
            path: { device_id: deviceId as string },
            query: {
              format_name: formatName as FormatName,
              width: width as number,
              height: height as number,
            },
          },
          signal,
        }),
        'Failed to fetch framerates',
      );
      return result.framerates ?? [];
    },
    [deviceId, formatName, width, height],
    {
      initial: [],
      enabled,
      // Suppress error noise for invalid resolution combinations the backend
      // rejects with 400/500 — UI silently shows empty list instead.
      onError: (err) => {
        const msg = err.message;
        if (msg.includes('400') || msg.includes('500')) return null;
        return msg;
      },
    },
  );

  return { framerates, loading, error };
}
