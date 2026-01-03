import { useState } from 'react';
import { useAudioDevices } from '../../hooks/useAudioDevices';

interface AudioDeviceSelectorProps {
  value: string;
  onChange: (device: string) => void;
  disabled?: boolean;
  error?: string;
  className?: string;
}

const CUSTOM_OPTION = '__custom__';

export function AudioDeviceSelector({
  value,
  onChange,
  disabled = false,
  error,
  className = ''
}: Readonly<AudioDeviceSelectorProps>) {
  const { devices, loading, error: loadError } = useAudioDevices();

  // Track if user manually selected custom mode from dropdown
  const [manualCustom, setManualCustom] = useState(false);
  const [customValue, setCustomValue] = useState('');

  // Derive isCustom: either user manually selected it, or value is not in the device list
  const valueIsCustom = !loading && value !== '' && !devices.some(d => d.alsa_device === value);
  const isCustom = manualCustom || valueIsCustom;

  // Display value for custom input
  const displayCustomValue = customValue || (valueIsCustom ? value : '');

  const renderContent = () => {
    if (loading) {
      return (
        <div className="flex items-center space-x-2 p-3 border border-gray-300 dark:border-gray-600 rounded-md bg-gray-50 dark:bg-gray-800">
          <div className="w-4 h-4 border-2 border-blue-500 border-t-transparent rounded-full animate-spin" />
          <span className="text-sm text-gray-600 dark:text-gray-300">Loading audio devices...</span>
        </div>
      );
    }
    
    if (loadError) {
      return (
        <div className="p-3 border border-red-300 dark:border-red-600 rounded-md bg-red-50 dark:bg-red-900/20">
          <p className="text-sm text-red-600 dark:text-red-400">Failed to load audio devices: {loadError}</p>
        </div>
      );
    }
    
    return (
      <>
        <div className={`${isCustom ? 'grid grid-cols-2 gap-2' : ''}`}>
          <select
            value={isCustom ? CUSTOM_OPTION : value}
            onChange={(e) => {
              if (e.target.value === CUSTOM_OPTION) {
                setManualCustom(true);
                setCustomValue('');
                onChange('');
              } else {
                setManualCustom(false);
                setCustomValue('');
                onChange(e.target.value);
              }
            }}
            className="block w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-800 dark:text-white"
            disabled={disabled}
          >
            <option value="">No Audio</option>
            {devices.map((device) => (
              <option key={device.alsa_device} value={device.alsa_device}>
                {device.card_name} - {device.device_name} ({device.alsa_device})
              </option>
            ))}
            <option value={CUSTOM_OPTION}>Custom...</option>
          </select>
          
          {isCustom && (
            <input
              type="text"
              value={displayCustomValue}
              onChange={(e) => {
                setCustomValue(e.target.value);
                onChange(e.target.value);
              }}
              placeholder="e.g., hw:0,0 or pulse"
              className="block w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-800 dark:text-white"
              disabled={disabled}
            />
          )}
        </div>
        <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {isCustom 
            ? 'Enter a custom ALSA device string (e.g., hw:0,0) or PulseAudio device'
            : 'Select an audio device to enable audio passthrough, or leave as "No Audio" for video-only stream'}
        </p>
      </>
    );
  };

  return (
    <div className={className}>
      <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
        Audio Device
      </label>
      {renderContent()}
      {error && (
        <p className="mt-1 text-sm text-red-600 dark:text-red-400">{error}</p>
      )}
    </div>
  );
}