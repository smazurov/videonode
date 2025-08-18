import { useEffect } from 'react';
import { Button } from '../Button';
import { useDeviceStore } from '../../hooks/useDeviceStore';
import { 
  useDeviceFormats
} from '../../hooks/useDeviceCapabilities';

type DeviceFormat = {
  format_name: string;
  original_name: string;
  emulated: boolean;
};

interface DeviceSelectorProps {
  value: string;
  onChange: (deviceId: string) => void;
  error?: string | undefined;
  disabled?: boolean;
  required?: boolean;
}

export function DeviceSelector({ 
  value, 
  onChange, 
  error, 
  disabled = false,
  required = false 
}: Readonly<DeviceSelectorProps>) {
  const devices = useDeviceStore((state) => state.devices);
  const loadingDevices = useDeviceStore((state) => state.loading);
  const deviceError = useDeviceStore((state) => state.error);
  
  const { formats, loading: loadingFormats, error: formatsError } = useDeviceFormats(value);
  
  // Load devices on mount only if store is empty
  useEffect(() => {
    if (devices.length === 0) {
      useDeviceStore.getState().fetchDevices();
    }
  }, [devices.length]);
  
  // Auto-select first device when devices are loaded
  useEffect(() => {
    if (devices.length > 0 && !value) {
      onChange(devices[0]?.device_id || '');
    }
  }, [devices, value, onChange]);
  
  const selectedDevice = devices.find(d => d.device_id === value);
  
  const renderDeviceSelection = () => {
    if (loadingDevices) {
      return (
        <div className="flex items-center space-x-2 p-3 border border-gray-300 dark:border-gray-600 rounded-md bg-gray-50 dark:bg-gray-800">
          <div className="w-4 h-4 border-2 border-blue-500 border-t-transparent rounded-full animate-spin" />
          <span className="text-sm text-gray-600 dark:text-gray-300">Loading devices...</span>
        </div>
      );
    }
    
    if (deviceError !== null) {
      return (
        <div className="space-y-2">
          <div className="p-3 border border-red-300 dark:border-red-600 rounded-md bg-red-50 dark:bg-red-900/20">
            <p className="text-sm text-red-600 dark:text-red-400">{deviceError}</p>
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
        <div className="p-3 border border-yellow-300 dark:border-yellow-600 rounded-md bg-yellow-50 dark:bg-yellow-900/20">
          <p className="text-sm text-yellow-700 dark:text-yellow-300">
            No devices found. Make sure your video capture devices are connected.
          </p>
        </div>
      );
    }
    
    return (
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="block w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-800 dark:text-white"
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
  
  const renderFormatsList = (formats: DeviceFormat[]) => (
    <div className="text-sm">
      <p className="text-gray-700 dark:text-gray-200 font-medium mb-1">Available Input Formats:</p>
      <div className="flex flex-wrap gap-1">
        {formats.map(format => (
          <span 
            key={format.format_name}
            className={`px-2 py-1 rounded text-xs ${
              format.emulated 
                ? 'bg-gray-100 dark:bg-gray-800 text-gray-600 dark:text-gray-400' 
                : 'bg-blue-100 dark:bg-blue-900 text-blue-700 dark:text-blue-300'
            }`}
            title={`${format.original_name}${format.emulated ? ' (Emulated)' : ''}`}
          >
            {format.format_name.toUpperCase()}
          </span>
        ))}
      </div>
    </div>
  );
  
  return (
    <div>
      <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
        Video Device {required && <span className="text-red-500">*</span>}
      </label>
      
      {renderDeviceSelection()}
      
      {error && (
        <p className="mt-1 text-sm text-red-600 dark:text-red-400">{error}</p>
      )}
      
      {selectedDevice && (
        <div className="mt-2 p-3 bg-gray-50 dark:bg-gray-800 rounded">
          <div className="space-y-2">
            <div className="text-sm">
              <p className="text-gray-600 dark:text-gray-300">
                <strong>Device:</strong> {selectedDevice.device_name}
              </p>
              <p className="text-gray-600 dark:text-gray-300">
                <strong>Capabilities:</strong> {selectedDevice.capabilities.join(', ')}
              </p>
            </div>
            
            {loadingFormats && (
              <div className="flex items-center space-x-2 text-sm">
                <div className="w-3 h-3 border-2 border-blue-500 border-t-transparent rounded-full animate-spin" />
                <span className="text-gray-600 dark:text-gray-300">Loading formats...</span>
              </div>
            )}
            
            {!loadingFormats && formats.length > 0 && renderFormatsList(formats)}
            
            {formatsError && (
              <p className="text-sm text-red-600 dark:text-red-400">
                Failed to load formats: {formatsError}
              </p>
            )}
          </div>
        </div>
      )}
    </div>
  );
}