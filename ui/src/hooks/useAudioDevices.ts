import { useEffect, useReducer } from 'react';
import type { components } from '../lib/api.generated';
import { api, unwrap } from '../lib/api';
import { useDeviceStore } from './useDeviceStore';

type AudioDevice = components["schemas"]["AudioDevice"];

interface State {
  devices: AudioDevice[];
  loading: boolean;
  error: string | null;
}

type Action =
  | { type: 'start' }
  | { type: 'done'; devices: AudioDevice[] }
  | { type: 'fail'; error: string };

function reducer(_state: State, action: Action): State {
  switch (action.type) {
    case 'start': return { devices: [], loading: true, error: null };
    case 'done': return { devices: action.devices, loading: false, error: null };
    case 'fail': return { devices: [], loading: false, error: action.error };
  }
}

const initial: State = { devices: [], loading: false, error: null };

export function useAudioDevices() {
  const devicesUpdatedAt = useDeviceStore((s) => s.lastUpdated);
  const [state, dispatch] = useReducer(reducer, initial);

  useEffect(() => {
    const controller = new AbortController();
    const signal = controller.signal;

    dispatch({ type: 'start' });

    api.GET("/api/devices/audio", { signal })
      .then((response) => {
        if (signal.aborted) return;
        const result = unwrap(response, 'Failed to load audio devices');
        dispatch({ type: 'done', devices: result.devices ?? [] });
      })
      .catch((error_: unknown) => {
        if (signal.aborted) return;
        const e = error_ instanceof Error ? error_ : new Error(String(error_));
        if (e.name === 'AbortError') return;
        dispatch({ type: 'fail', error: e.message });
      });

    return () => {
      controller.abort();
    };
  }, [devicesUpdatedAt]);

  return { devices: state.devices, loading: state.loading, error: state.error };
}
