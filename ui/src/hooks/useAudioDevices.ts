import type { components } from '../lib/api.generated';
import { api, unwrap } from '../lib/api';
import { useAbortableQuery } from './useAbortableQuery';

type AudioDevice = components["schemas"]["AudioDevice"];

export function useAudioDevices() {
  const { data: devices, loading, error } = useAbortableQuery<AudioDevice[]>(
    async (signal) => {
      const result = unwrap(
        await api.GET("/api/devices/audio", { signal }),
        'Failed to load audio devices',
      );
      return result.devices ?? [];
    },
    [],
    { initial: [] },
  );

  return { devices, loading, error };
}
