import type { components } from '../lib/api.generated';
import { api, unwrap } from '../lib/api';
import { useAbortableQuery } from './useAbortableQuery';
import { useDeviceStore } from './useDeviceStore';

type AudioDevice = components["schemas"]["AudioDevice"];

export function useAudioDevices() {
  // Re-fetch when the device list refreshes (device-discovery event or the
  // periodic fallback poll) so hot-plugged audio devices surface without a
  // manual reload.
  const devicesUpdatedAt = useDeviceStore((s) => s.lastUpdated);
  const { data: devices, loading, error } = useAbortableQuery<AudioDevice[]>(
    async (signal) => {
      const result = unwrap(
        await api.GET("/api/devices/audio", { signal }),
        'Failed to load audio devices',
      );
      return result.devices ?? [];
    },
    [devicesUpdatedAt],
    { initial: [] },
  );

  return { devices, loading, error };
}
