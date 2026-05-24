import { useEffect } from 'react';
import { Button } from '../Button';
import { Spinner } from '../Spinner';
import { useDeviceStore } from '../../hooks/useDeviceStore';

// Adapted from StreamCreation/DeviceSelector (deleted in U14). Trimmed to
// the source form's needs: pick one device, surface its formats read-only.

interface SourceDeviceFieldProps {
  value: string;
  onChange: (deviceId: string) => void;
  error?: string;
  disabled?: boolean;
  required?: boolean;
}

const selectClasses =
  'block w-full px-3 py-2 border border-border rounded-md shadow-sm bg-surface text-fg focus:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:border-accent';

export function SourceDeviceField({
  value,
  onChange,
  error,
  disabled = false,
  required = false,
}: Readonly<SourceDeviceFieldProps>) {
  const devices = useDeviceStore((state) => state.devices);
  const loadingDevices = useDeviceStore((state) => state.loading);
  const deviceError = useDeviceStore((state) => state.error);

  useEffect(() => {
    if (devices.length === 0) {
      useDeviceStore.getState().fetchDevices();
    }
  }, [devices.length]);

  useEffect(() => {
    if (devices.length > 0 && !value) {
      const first = devices[0]?.device_id;
      if (first) onChange(first);
    }
  }, [devices, value, onChange]);

  const selectedDevice = devices.find((d) => d.device_id === value);

  const renderSelect = () => {
    if (loadingDevices) {
      return (
        <div className="flex items-center space-x-2 p-3 border border-border rounded-md bg-surface-muted">
          <Spinner size="sm" />
          <span className="text-sm text-fg-muted">Loading devices...</span>
        </div>
      );
    }

    if (deviceError !== null) {
      return (
        <div className="space-y-2">
          <div className="p-3 border border-danger rounded-md bg-danger-soft">
            <p className="text-sm text-danger-soft-fg">{deviceError}</p>
          </div>
          <Button
            type="button"
            theme="light"
            size="SM"
            onClick={() => useDeviceStore.getState().fetchDevices()}
            text="Retry"
          />
        </div>
      );
    }

    if (devices.length === 0) {
      return (
        <div className="p-3 border border-warning rounded-md bg-warning-soft">
          <p className="text-sm text-warning-soft-fg">
            No devices found. Make sure your video capture devices are connected.
          </p>
        </div>
      );
    }

    return (
      <select
        aria-label="Video device"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className={selectClasses}
        disabled={disabled}
        required={required}
      >
        <option value="">Select a device...</option>
        {devices.map((device) => (
          <option key={device.device_id} value={device.device_id}>
            {device.device_name} ({device.device_path})
          </option>
        ))}
      </select>
    );
  };

  return (
    <div>
      <label className="block text-sm font-medium text-fg mb-2">
        Video Device {required && <span className="text-danger">*</span>}
      </label>

      {renderSelect()}

      {error && <p className="mt-1 text-sm text-danger-soft-fg">{error}</p>}

      {selectedDevice && (
        <div className="mt-2 p-3 bg-surface-muted rounded">
          <div className="text-sm">
            <p className="text-fg-muted">
              <strong>Device:</strong> {selectedDevice.device_name}
            </p>
            <p className="text-fg-muted">
              <strong>Capabilities:</strong>{' '}
              {(selectedDevice.capabilities ?? []).join(', ')}
            </p>
          </div>
        </div>
      )}
    </div>
  );
}
