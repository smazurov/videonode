import { useEffect, useReducer } from 'react';
import type { components } from '../lib/api.generated';
import { api, unwrap } from '../lib/api';

type FormatInfo = components["schemas"]["FormatInfo"];
type FormatName = FormatInfo["format_name"];
type Resolution = components["schemas"]["Resolution"];
type Framerate = components["schemas"]["Framerate"];

export interface DeviceFrameratesResult {
  framerates: Framerate[];
  loading: boolean;
  error: string | null;
}

type Action<T> =
  | { type: 'start' }
  | { type: 'done'; data: T }
  | { type: 'fail'; initial: T; error: string | null }
  | { type: 'reset'; initial: T };

interface State<T> {
  data: T;
  loading: boolean;
  error: string | null;
}

function fetchReducer<T>(state: State<T>, action: Action<T>): State<T> {
  switch (action.type) {
    case 'start': return { data: state.data, loading: true, error: null };
    case 'done': return { data: action.data, loading: false, error: null };
    case 'fail': return { data: action.initial, loading: false, error: action.error };
    case 'reset': return { data: action.initial, loading: false, error: null };
  }
}

export function useDeviceFormats(deviceId: string) {
  const [state, dispatch] = useReducer(fetchReducer<FormatInfo[]>, {
    data: [],
    loading: false,
    error: null,
  });

  useEffect(() => {
    if (!deviceId) {
      dispatch({ type: 'reset', initial: [] });
      return;
    }

    const controller = new AbortController();
    const signal = controller.signal;

    dispatch({ type: 'start' });

    api.GET("/api/devices/{device_id}/formats", {
      params: { path: { device_id: deviceId } },
      signal,
    })
      .then((response) => {
        if (signal.aborted) return;
        const result = unwrap(response, 'Failed to fetch formats');
        dispatch({ type: 'done', data: result.formats ?? [] });
      })
      .catch((error_: unknown) => {
        if (signal.aborted) return;
        const e = error_ instanceof Error ? error_ : new Error(String(error_));
        if (e.name === 'AbortError') return;
        dispatch({ type: 'fail', initial: [], error: e.message });
      });

    return () => {
      controller.abort();
    };
  }, [deviceId]);

  return { formats: state.data, loading: state.loading, error: state.error };
}

export function useDeviceResolutions(deviceId: string, formatName: FormatName | undefined) {
  const enabled = !!deviceId && !!formatName;
  const [state, dispatch] = useReducer(fetchReducer<Resolution[]>, {
    data: [],
    loading: false,
    error: null,
  });

  useEffect(() => {
    if (!enabled) {
      dispatch({ type: 'reset', initial: [] });
      return;
    }

    const controller = new AbortController();
    const signal = controller.signal;

    dispatch({ type: 'start' });

    api.GET("/api/devices/{device_id}/resolutions", {
      params: {
        path: { device_id: deviceId },
        query: { format_name: formatName as FormatName },
      },
      signal,
    })
      .then((response) => {
        if (signal.aborted) return;
        const result = unwrap(response, 'Failed to fetch resolutions');
        dispatch({ type: 'done', data: result.resolutions ?? [] });
      })
      .catch((error_: unknown) => {
        if (signal.aborted) return;
        const e = error_ instanceof Error ? error_ : new Error(String(error_));
        if (e.name === 'AbortError') return;
        dispatch({ type: 'fail', initial: [], error: e.message });
      });

    return () => {
      controller.abort();
    };
  }, [deviceId, enabled, formatName]);

  return { resolutions: state.data, loading: state.loading, error: state.error };
}

export function useDeviceFramerates(
  deviceId: string | undefined,
  formatName: FormatName | undefined,
  width: number | undefined,
  height: number | undefined,
): DeviceFrameratesResult {
  const enabled = !!deviceId && !!formatName && !!width && !!height;
  const [state, dispatch] = useReducer(fetchReducer<Framerate[]>, {
    data: [],
    loading: false,
    error: null,
  });

  useEffect(() => {
    if (!enabled) {
      dispatch({ type: 'reset', initial: [] });
      return;
    }

    const controller = new AbortController();
    const signal = controller.signal;

    dispatch({ type: 'start' });

    api.GET("/api/devices/{device_id}/framerates", {
      params: {
        path: { device_id: deviceId as string },
        query: {
          format_name: formatName as FormatName,
          width: width as number,
          height: height as number,
        },
      },
      signal,
    })
      .then((response) => {
        if (signal.aborted) return;
        const result = unwrap(response, 'Failed to fetch framerates');
        dispatch({ type: 'done', data: result.framerates ?? [] });
      })
      .catch((error_: unknown) => {
        if (signal.aborted) return;
        const e = error_ instanceof Error ? error_ : new Error(String(error_));
        if (e.name === 'AbortError') return;
        const msg = e.message;
        if (msg.includes('400') || msg.includes('500')) {
          dispatch({ type: 'fail', initial: [], error: null });
        } else {
          dispatch({ type: 'fail', initial: [], error: msg });
        }
      });

    return () => {
      controller.abort();
    };
  }, [deviceId, enabled, formatName, height, width]);

  return { framerates: state.data, loading: state.loading, error: state.error };
}
