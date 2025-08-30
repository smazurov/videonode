import { FormEvent } from 'react';
import { Card } from '../Card';
import { Button } from '../Button';
import { InputField } from '../InputField';
import { useDeviceStore } from '../../hooks/useDeviceStore';
import { useStreamCreation } from '../../hooks/useStreamCreation';
import { RESOLUTION_LABELS } from './constants';
import { AdvancedOptions } from './AdvancedOptions';

const MANUAL_FPS_OPTIONS = [
  { value: '24', label: '24 FPS' },
  { value: '25', label: '25 FPS' },
  { value: '30', label: '30 FPS' },
  { value: '50', label: '50 FPS' },
  { value: '60', label: '60 FPS' },
];

interface StreamCreationFormProps {
  onCreateStream: () => Promise<void>;
  onCancel?: () => void;
  className?: string;
}

export function StreamCreationForm({
  onCreateStream,
  onCancel,
  className = ''
}: Readonly<StreamCreationFormProps>) {
  const devices = useDeviceStore((state) => state.devices);
  const {
    state,
    formats,
    resolutions,
    framerates,
    loading,
    actions
  } = useStreamCreation();
  
  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    
    const success = await actions.createStream();
    if (success) {
      // The stream data is already sent by createStream
      // Just notify parent that creation is complete
      await onCreateStream();
      actions.reset();
    }
  };
  
  const isCreating = state.status === 'creating';
  
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
            value={state.streamId}
            onChange={(e) => actions.setStreamId(e.target.value)}
            placeholder="my-stream-001"
            required
            disabled={isCreating}
            {...(state.errors.streamId ? { error: state.errors.streamId } : {})}
          />
          
          {/* Device Selection */}
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
              Video Device <span className="text-red-500">*</span>
            </label>
            <select
              value={state.deviceId}
              onChange={(e) => actions.selectDevice(e.target.value)}
              className="block w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-800 dark:text-white"
              disabled={isCreating || devices.length === 0}
              required
            >
              <option value="">Select device...</option>
              {devices.map((device) => (
                <option key={device.device_id} value={device.device_id}>
                  {device.device_name} ({device.device_path})
                </option>
              ))}
            </select>
            {state.errors.deviceId && (
              <p className="mt-1 text-sm text-red-600 dark:text-red-400">{state.errors.deviceId}</p>
            )}
          </div>
          
          {/* Format, Resolution, and Framerate Configuration */}
          {state.deviceId && (
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              {/* Input Format */}
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                  Input Format <span className="text-red-500">*</span>
                </label>
                {loading.formats ? (
                  <div className="flex items-center space-x-2 p-3 border border-gray-300 dark:border-gray-600 rounded-md bg-gray-50 dark:bg-gray-800">
                    <div className="w-4 h-4 border-2 border-blue-500 border-t-transparent rounded-full animate-spin" />
                    <span className="text-sm text-gray-600 dark:text-gray-300">Loading...</span>
                  </div>
                ) : (
                  <select
                    value={state.format}
                    onChange={(e) => actions.selectFormat(e.target.value)}
                    className="block w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-800 dark:text-white"
                    disabled={isCreating || formats.length === 0}
                    required
                  >
                    <option value="">Select format...</option>
                    {formats.map((format) => (
                      <option key={format.format_name} value={format.format_name}>
                        {format.format_name.toUpperCase()} - {format.original_name}
                      </option>
                    ))}
                  </select>
                )}
                {state.errors.format && (
                  <p className="mt-1 text-sm text-red-600 dark:text-red-400">{state.errors.format}</p>
                )}
              </div>
              
              {/* Resolution */}
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                  Resolution
                </label>
                {loading.resolutions && state.format ? (
                  <div className="flex items-center space-x-2 p-3 border border-gray-300 dark:border-gray-600 rounded-md bg-gray-50 dark:bg-gray-800">
                    <div className="w-4 h-4 border-2 border-blue-500 border-t-transparent rounded-full animate-spin" />
                    <span className="text-sm text-gray-600 dark:text-gray-300">Loading...</span>
                  </div>
                ) : (
                  <select
                    value={state.width && state.height ? `${state.width}x${state.height}` : ''}
                    onChange={(e) => {
                      if (e.target.value === '') {
                        // User selected "Auto"
                        actions.selectResolution(0, 0);
                      } else {
                        const [w, h] = e.target.value.split('x').map(Number);
                        if (w && h) {
                          actions.selectResolution(w, h);
                        }
                      }
                    }}
                    className="block w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-800 dark:text-white"
                    disabled={isCreating}
                  >
                    <option value="">Auto</option>
                    {!state.format && <option disabled>Select format first to see resolutions</option>}
                    {resolutions.map((res) => {
                      const resString = `${res.width}x${res.height}`;
                      const label = RESOLUTION_LABELS[resString] 
                        ? `${resString} (${RESOLUTION_LABELS[resString]})` 
                        : resString;
                      
                      return (
                        <option key={resString} value={resString}>
                          {label}
                        </option>
                      );
                    })}
                  </select>
                )}
                {state.errors.resolution && (
                  <p className="mt-1 text-sm text-red-600 dark:text-red-400">{state.errors.resolution}</p>
                )}
              </div>
              
              {/* Framerate */}
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                  Framerate
                </label>
                {loading.framerates && state.width > 0 && state.height > 0 ? (
                  <div className="flex items-center space-x-2 p-3 border border-gray-300 dark:border-gray-600 rounded-md bg-gray-50 dark:bg-gray-800">
                    <div className="w-4 h-4 border-2 border-blue-500 border-t-transparent rounded-full animate-spin" />
                    <span className="text-sm text-gray-600 dark:text-gray-300">Loading...</span>
                  </div>
                ) : (
                  <select
                    value={state.framerate?.toString() || ''}
                    onChange={(e) => {
                      if (e.target.value === '') {
                        actions.selectFramerate(0); // 0 means auto
                      } else {
                        actions.selectFramerate(parseInt(e.target.value, 10));
                      }
                    }}
                    className="block w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-800 dark:text-white"
                    disabled={isCreating}
                  >
                    <option value="">Auto</option>
                    {/* Use API framerates if available and resolution is specific, otherwise use manual options */}
                    {framerates.length > 0 && state.width > 0 && state.height > 0 ? (
                      framerates.map((rate) => {
                        const fpsValue = Math.round(rate.fps);
                        return (
                          <option key={`${rate.numerator}/${rate.denominator}`} value={fpsValue.toString()}>
                            {fpsValue} FPS ({rate.numerator}/{rate.denominator})
                          </option>
                        );
                      })
                    ) : (
                      MANUAL_FPS_OPTIONS.map(opt => (
                        <option key={opt.value} value={opt.value}>{opt.label}</option>
                      ))
                    )}
                  </select>
                )}
                {state.errors.framerate && (
                  <p className="mt-1 text-sm text-red-600 dark:text-red-400">{state.errors.framerate}</p>
                )}
              </div>
            </div>
          )}
          
          {/* Codec and Bitrate Configuration */}
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                Codec <span className="text-red-500">*</span>
              </label>
              <select
                value={state.codec}
                onChange={(e) => actions.setCodec(e.target.value)}
                className="block w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-800 dark:text-white"
                disabled={isCreating}
                required
              >
                <option value="h264">H.264</option>
                <option value="h265">H.265 (HEVC)</option>
              </select>
              {state.errors.codec && (
                <p className="mt-1 text-sm text-red-600 dark:text-red-400">{state.errors.codec}</p>
              )}
            </div>
            
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                Bitrate
              </label>
              <div className="relative">
                <input
                  type="number"
                  value={state.bitrate || 2}
                  onChange={(e) => {
                    const mbps = parseFloat(e.target.value);
                    if (!isNaN(mbps) && mbps > 0) {
                      actions.setBitrate(mbps); // Keep as Mbps
                    } else if (e.target.value === '') {
                      actions.setBitrate(2); // Default to 2 Mbps
                    }
                  }}
                  placeholder="2.0"
                  step="0.1"
                  min="0.1"
                  max="50"
                  className="block w-full pl-3 pr-16 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-800 dark:text-white"
                  disabled={isCreating}
                />
                <div className="absolute inset-y-0 right-0 pr-3 flex items-center pointer-events-none">
                  <span className="text-gray-500 dark:text-gray-400 sm:text-sm">Mbps</span>
                </div>
              </div>
              {state.errors.bitrate && (
                <p className="mt-1 text-sm text-red-600 dark:text-red-400">{state.errors.bitrate}</p>
              )}
            </div>
          </div>
          
          {/* Advanced Options */}
          <AdvancedOptions
            selectedOptions={state.options}
            onOptionsChange={actions.setOptions}
            disabled={isCreating}
            className="mt-4"
          />
          
          {/* Error message */}
          {state.errors.submit && (
            <div className="p-3 border border-red-300 dark:border-red-600 rounded-md bg-red-50 dark:bg-red-900/20">
              <p className="text-sm text-red-600 dark:text-red-400">{state.errors.submit}</p>
            </div>
          )}
          
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
              disabled={isCreating || !state.isValid}
              text={isCreating ? 'Creating...' : 'Create Stream'}
            />
          </div>
        </form>
      </Card.Content>
    </Card>
  );
}