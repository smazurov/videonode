import { useState } from 'react';
import { useAudioDevices } from '../../hooks/useAudioDevices';
import { Spinner } from '../Spinner';

interface AudioDeviceSelectorProps {
  value: string;
  onChange: (device: string) => void;
  disabled?: boolean;
  error?: string;
  className?: string;
}

const CUSTOM_OPTION = '__custom__';

const inputClasses =
  'block w-full px-3 py-2 border border-border rounded-md shadow-sm bg-surface text-fg focus:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:border-accent';

export function AudioDeviceSelector({
  value,
  onChange,
  disabled = false,
  error,
  className = ''
}: Readonly<AudioDeviceSelectorProps>) {
  const { devices, loading, error: loadError } = useAudioDevices();

  const [manualCustom, setManualCustom] = useState(false);
  const [customValue, setCustomValue] = useState('');

  const valueIsCustom = !loading && value !== '' && !devices.some(d => d.alsa_device === value);
  const isCustom = manualCustom || valueIsCustom;

  const displayCustomValue = customValue || (valueIsCustom ? value : '');

  const renderContent = () => {
    if (loading) {
      return (
        <div className="flex items-center space-x-2 p-3 border border-border rounded-md bg-surface-muted">
          <Spinner size="sm" />
          <span className="text-sm text-fg-muted">Loading audio devices...</span>
        </div>
      );
    }

    if (loadError) {
      return (
        <div className="p-3 border border-danger rounded-md bg-danger-soft">
          <p className="text-sm text-danger-soft-fg">Failed to load audio devices: {loadError}</p>
        </div>
      );
    }

    return (
      <>
        <div className={`${isCustom ? 'grid grid-cols-2 gap-2' : ''}`}>
          <select
            aria-label="Audio device"
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
            className={inputClasses}
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
              aria-label="Custom audio device"
              value={displayCustomValue}
              onChange={(e) => {
                setCustomValue(e.target.value);
                onChange(e.target.value);
              }}
              placeholder="e.g., hw:0,0 or pulse"
              className={inputClasses}
              disabled={disabled}
            />
          )}
        </div>
        <p className="mt-1 text-xs text-fg-subtle">
          {isCustom
            ? 'Enter a custom ALSA device string (e.g., hw:0,0) or PulseAudio device'
            : 'Select an audio device to enable audio passthrough, or leave as "No Audio" for video-only stream'}
        </p>
      </>
    );
  };

  return (
    <div className={className}>
      <label className="block text-sm font-medium text-fg mb-2">Audio Device</label>
      {renderContent()}
      {error && <p className="mt-1 text-sm text-danger-soft-fg">{error}</p>}
    </div>
  );
}
