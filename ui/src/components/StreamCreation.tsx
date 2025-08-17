import { useState, useEffect } from 'react';
import { Card } from './Card';
import { Button } from './Button';
import { InputField } from './InputField';
import { StreamRequestData } from '../lib/api';
import { useDeviceStore } from '../hooks/useDeviceStore';

interface StreamCreationProps {
  onCreateStream: (streamData: StreamRequestData) => Promise<void>;
  onCancel?: () => void;
  isCreating?: boolean;
  className?: string;
}

const COMMON_CODECS = [
  { value: 'h264', label: 'H.264 (Recommended)' },
  { value: 'h265', label: 'H.265/HEVC' },
  { value: 'mjpeg', label: 'MJPEG' },
  { value: 'libx264', label: 'libx264' },
  { value: 'libx265', label: 'libx265' }
];

const COMMON_RESOLUTIONS = [
  { value: '', label: 'Auto (Device Default)' },
  { value: '3840x2160', label: '4K (3840x2160)' },
  { value: '1920x1080', label: 'Full HD (1920x1080)' },
  { value: '1280x720', label: 'HD (1280x720)' },
  { value: '640x480', label: 'VGA (640x480)' }
];

const COMMON_FRAMERATES = [
  { value: '', label: 'Auto' },
  { value: '60', label: '60 FPS' },
  { value: '30', label: '30 FPS' },
  { value: '24', label: '24 FPS' },
  { value: '15', label: '15 FPS' }
];

const COMMON_BITRATES = [
  { value: '', label: 'Auto' },
  { value: '8000', label: '8 Mbps (High Quality)' },
  { value: '4000', label: '4 Mbps (Medium Quality)' },
  { value: '2000', label: '2 Mbps (Standard)' },
  { value: '1000', label: '1 Mbps (Low Bandwidth)' }
];

export function StreamCreation({ 
  onCreateStream, 
  onCancel, 
  isCreating = false, 
  className = '' 
}: Readonly<StreamCreationProps>) {
  const devices = useDeviceStore((state) => state.devices);
  const loadingDevices = useDeviceStore((state) => state.loading);
  const deviceError = useDeviceStore((state) => state.error);
  
  const [formData, setFormData] = useState<StreamRequestData>({
    stream_id: '',
    device_id: '',
    codec: 'h264'
  });
  
  const [formErrors, setFormErrors] = useState<Record<string, string>>({});

  // Auto-select first device when devices are loaded
  useEffect(() => {
    if (devices.length > 0 && !formData.device_id) {
      setFormData(prev => ({
        ...prev,
        device_id: devices[0]?.device_id || ''
      }));
    }
  }, [devices, formData.device_id]);

  // Load devices on mount only if store is empty
  useEffect(() => {
    if (devices.length === 0) {
      useDeviceStore.getState().fetchDevices();
    }
  }, [devices.length]); // Include devices.length dependency

  const validateForm = (): boolean => {
    const errors: Record<string, string> = {};
    
    if (!formData.stream_id.trim()) {
      errors.stream_id = 'Stream ID is required';
    } else if (!/^[\w-]+$/.test(formData.stream_id)) {
      errors.stream_id = 'Stream ID can only contain letters, numbers, dashes, and underscores';
    }
    
    if (!formData.device_id) {
      errors.device_id = 'Device selection is required';
    }
    
    if (!formData.codec) {
      errors.codec = 'Codec selection is required';
    }
    
    if (formData.bitrate && (formData.bitrate < 100 || formData.bitrate > 50000)) {
      errors.bitrate = 'Bitrate must be between 100 and 50000 kbps';
    }
    
    if (formData.width && (formData.width < 160 || formData.width > 7680)) {
      errors.width = 'Width must be between 160 and 7680 pixels';
    }
    
    if (formData.height && (formData.height < 120 || formData.height > 4320)) {
      errors.height = 'Height must be between 120 and 4320 pixels';
    }
    
    if (formData.framerate && (formData.framerate < 1 || formData.framerate > 120)) {
      errors.framerate = 'Framerate must be between 1 and 120 FPS';
    }
    
    setFormErrors(errors);
    return Object.keys(errors).length === 0;
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    
    if (!validateForm()) {
      return;
    }
    
    try {
      await onCreateStream(formData);
    } catch (error) {
      console.error('Failed to create stream:', error);
    }
  };

  const handleInputChange = (field: keyof StreamRequestData, value: string) => {
    const numericFields = ['bitrate', 'width', 'height', 'framerate'] as const;
    
    let processedValue: string | number | undefined;
    if (value === '') {
      processedValue = undefined;
    } else if (numericFields.includes(field as typeof numericFields[number])) {
      processedValue = parseInt(value, 10);
    } else {
      processedValue = value;
    }
    
    setFormData(prev => ({
      ...prev,
      [field]: processedValue
    }));
    
    // Clear field error when user starts typing
    if (formErrors[field]) {
      setFormErrors(prev => ({
        ...prev,
        [field]: ''
      }));
    }
  };

  const handleResolutionChange = (resolution: string) => {
    if (resolution === '') {
      setFormData(prev => {
        const newData = { ...prev };
        delete newData.width;
        delete newData.height;
        return newData as StreamRequestData;
      });
    } else {
      const [width, height] = resolution.split('x').map(Number);
      setFormData(prev => ({
        ...prev,
        ...(width && { width }),
        ...(height && { height })
      }));
    }
  };

  const selectedDevice = devices.find(d => d.device_id === formData.device_id);
  const currentResolution = formData.width && formData.height ? `${formData.width}x${formData.height}` : '';

  return (
    <Card className={className}>
      <Card.Header>
        <h3 className="text-lg font-semibold text-gray-900 dark:text-white">
          Create New Stream
        </h3>
        <p className="text-sm text-gray-600 dark:text-gray-300 mt-1">
          Configure a new video stream from your capture devices
        </p>
      </Card.Header>
      
      <Card.Content>
        <form onSubmit={handleSubmit} className="space-y-6">
          {/* Stream ID */}
          <InputField
            label="Stream ID"
            type="text"
            value={formData.stream_id}
            onChange={(e) => handleInputChange('stream_id', e.target.value)}
            placeholder="my-stream-001"
            {...(formErrors.stream_id && { error: formErrors.stream_id })}
            disabled={isCreating}
            required
          />
          
          {/* Device Selection */}
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
              Video Device <span className="text-red-500">*</span>
            </label>
            
{(() => {
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
                  value={formData.device_id}
                  onChange={(e) => handleInputChange('device_id', e.target.value)}
                  className="block w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-800 dark:text-white"
                  disabled={isCreating}
                  required
                >
                  <option value="">Select a device...</option>
                  {devices.map((device) => (
                    <option key={device.device_id} value={device.device_id}>
                      {device.device_name} ({device.device_path})
                    </option>
                  ))}
                </select>
              );
            })()}
            
            {formErrors.device_id && (
              <p className="mt-1 text-sm text-red-600 dark:text-red-400">{formErrors.device_id}</p>
            )}
            
            {selectedDevice && (
              <div className="mt-2 p-2 bg-gray-50 dark:bg-gray-800 rounded text-sm">
                <p className="text-gray-600 dark:text-gray-300">
                  <strong>Device:</strong> {selectedDevice.device_name}
                </p>
                <p className="text-gray-600 dark:text-gray-300">
                  <strong>Capabilities:</strong> {selectedDevice.capabilities.join(', ')}
                </p>
              </div>
            )}
          </div>
          
          {/* Codec Selection */}
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
              Video Codec <span className="text-red-500">*</span>
            </label>
            <select
              value={formData.codec}
              onChange={(e) => handleInputChange('codec', e.target.value)}
              className="block w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-800 dark:text-white"
              disabled={isCreating}
              required
            >
              {COMMON_CODECS.map((codec) => (
                <option key={codec.value} value={codec.value}>
                  {codec.label}
                </option>
              ))}
            </select>
            {formErrors.codec && (
              <p className="mt-1 text-sm text-red-600 dark:text-red-400">{formErrors.codec}</p>
            )}
          </div>
          
          {/* Resolution Selection */}
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
              Resolution
            </label>
            <select
              value={currentResolution}
              onChange={(e) => handleResolutionChange(e.target.value)}
              className="block w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-800 dark:text-white"
              disabled={isCreating}
            >
              {COMMON_RESOLUTIONS.map((resolution) => (
                <option key={resolution.value} value={resolution.value}>
                  {resolution.label}
                </option>
              ))}
            </select>
          </div>
          
          {/* Advanced Options */}
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                Bitrate (kbps)
              </label>
              <select
                value={formData.bitrate?.toString() || ''}
                onChange={(e) => handleInputChange('bitrate', e.target.value)}
                className="block w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-800 dark:text-white"
                disabled={isCreating}
              >
                {COMMON_BITRATES.map((bitrate) => (
                  <option key={bitrate.value} value={bitrate.value}>
                    {bitrate.label}
                  </option>
                ))}
              </select>
              {formErrors.bitrate && (
                <p className="mt-1 text-sm text-red-600 dark:text-red-400">{formErrors.bitrate}</p>
              )}
            </div>
            
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                Framerate (FPS)
              </label>
              <select
                value={formData.framerate?.toString() || ''}
                onChange={(e) => handleInputChange('framerate', e.target.value)}
                className="block w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-800 dark:text-white"
                disabled={isCreating}
              >
                {COMMON_FRAMERATES.map((framerate) => (
                  <option key={framerate.value} value={framerate.value}>
                    {framerate.label}
                  </option>
                ))}
              </select>
              {formErrors.framerate && (
                <p className="mt-1 text-sm text-red-600 dark:text-red-400">{formErrors.framerate}</p>
              )}
            </div>
          </div>
          
          {/* Action Buttons */}
          <div className="flex justify-end space-x-3 pt-4 border-t border-gray-200 dark:border-gray-700">
            {onCancel && (
              <Button
                type="button"
                theme="light"
                size="MD"
                onClick={onCancel}
                disabled={isCreating}
                text="Cancel"
              />
            )}
            
            <Button
              type="submit"
              theme="primary"
              size="MD"
              disabled={isCreating || devices.length === 0}
              text={isCreating ? 'Creating...' : 'Create Stream'}
            />
          </div>
        </form>
      </Card.Content>
    </Card>
  );
}