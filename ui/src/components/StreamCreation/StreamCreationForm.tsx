import { useState, useEffect } from 'react';
import { Card } from '../Card';
import { Button } from '../Button';
import { InputField } from '../InputField';
import { StreamRequestData } from '../../lib/api';
import { useStreamForm } from '../../hooks/useStreamForm';
import { useDeviceAutoSelect } from '../../hooks/useDeviceAutoSelect';
import { useDeviceStore } from '../../hooks/useDeviceStore';
import { DeviceSelector } from './DeviceSelector';
import { FormatConfiguration } from './FormatConfiguration';
import { CodecConfiguration } from './CodecConfiguration';

interface StreamCreationFormProps {
  onCreateStream: (streamData: StreamRequestData) => Promise<void>;
  onCancel?: () => void;
  isCreating?: boolean;
  className?: string;
}

export function StreamCreationForm({ 
  onCreateStream, 
  onCancel, 
  isCreating = false, 
  className = '' 
}: Readonly<StreamCreationFormProps>) {
  const devices = useDeviceStore((state) => state.devices);
  const [selectedFormat, setSelectedFormat] = useState<string>('');
  
  const {
    formData,
    formErrors,
    setFormData,
    updateField,
    updateResolution,
    validateForm
  } = useStreamForm();
  
  // Use auto-selection hook
  useDeviceAutoSelect({
    deviceId: formData.device_id,
    selectedFormat,
    formData,
    setSelectedFormat,
    setFormData
  });
  
  // Auto-select first device when devices are loaded
  useEffect(() => {
    if (devices.length > 0 && !formData.device_id) {
      updateField('device_id', devices[0]?.device_id || '');
    }
  }, [devices, formData.device_id, updateField]);
  
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
    
    updateField(field, processedValue);
  };
  
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
          <DeviceSelector
            value={formData.device_id}
            onChange={(deviceId) => updateField('device_id', deviceId)}
            error={formErrors.device_id || undefined}
            disabled={isCreating}
            required
          />
          
          {/* Format, Resolution, and Framerate Configuration */}
          <FormatConfiguration
            deviceId={formData.device_id}
            selectedFormat={selectedFormat}
            width={formData.width}
            height={formData.height}
            framerate={formData.framerate}
            onFormatChange={(format) => {
              setSelectedFormat(format);
              updateField('input_format', format);
            }}
            onResolutionChange={updateResolution}
            onFramerateChange={(fps) => updateField('framerate', fps)}
            disabled={isCreating}
            errors={{
              framerate: formErrors.framerate || undefined,
              width: formErrors.width || undefined,
              height: formErrors.height || undefined
            }}
          />
          
          {/* Codec and Bitrate Configuration */}
          <CodecConfiguration
            codec={formData.codec}
            bitrate={formData.bitrate}
            onCodecChange={(codec) => updateField('codec', codec)}
            onBitrateChange={(bitrate) => updateField('bitrate', bitrate)}
            disabled={isCreating}
            errors={{
              codec: formErrors.codec || undefined,
              bitrate: formErrors.bitrate || undefined
            }}
          />
          
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