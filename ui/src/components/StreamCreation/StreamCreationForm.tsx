import { FormEvent } from 'react';
import { Card } from '../Card';
import { Button } from '../Button';
import { InputField } from '../InputField';
import { useDeviceStore } from '../../hooks/useDeviceStore';
import { useStreamCreation } from '../../hooks/useStreamCreation';
import { RESOLUTION_LABELS, COMMON_BITRATES } from './constants';

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
                {(() => {
                  if (!state.format) {
                    return (
                      <select
                        disabled
                        className="block w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm bg-gray-100 dark:bg-gray-900 cursor-not-allowed"
                      >
                        <option>Select format first</option>
                      </select>
                    );
                  }
                  if (loading.resolutions) {
                    return (
                      <div className="flex items-center space-x-2 p-3 border border-gray-300 dark:border-gray-600 rounded-md bg-gray-50 dark:bg-gray-800">
                        <div className="w-4 h-4 border-2 border-blue-500 border-t-transparent rounded-full animate-spin" />
                        <span className="text-sm text-gray-600 dark:text-gray-300">Loading...</span>
                      </div>
                    );
                  }
                  return (
                  <select
                    value={state.width && state.height ? `${state.width}x${state.height}` : ''}
                    onChange={(e) => {
                      if (e.target.value) {
                        const [w, h] = e.target.value.split('x').map(Number);
                        if (w && h) {
                          actions.selectResolution(w, h);
                        }
                      }
                    }}
                    className="block w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-800 dark:text-white"
                    disabled={isCreating || resolutions.length === 0}
                  >
                    <option value="">Auto</option>
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
                  );
                })()}
                {state.errors.resolution && (
                  <p className="mt-1 text-sm text-red-600 dark:text-red-400">{state.errors.resolution}</p>
                )}
              </div>
              
              {/* Framerate */}
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                  Framerate
                </label>
                {(() => {
                  if (!state.width || !state.height) {
                    return (
                      <select
                        disabled
                        className="block w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm bg-gray-100 dark:bg-gray-900 cursor-not-allowed"
                      >
                        <option>Select resolution first</option>
                      </select>
                    );
                  }
                  if (loading.framerates) {
                    return (
                      <div className="flex items-center space-x-2 p-3 border border-gray-300 dark:border-gray-600 rounded-md bg-gray-50 dark:bg-gray-800">
                        <div className="w-4 h-4 border-2 border-blue-500 border-t-transparent rounded-full animate-spin" />
                        <span className="text-sm text-gray-600 dark:text-gray-300">Loading...</span>
                      </div>
                    );
                  }
                  return (
                  <select
                    value={state.framerate?.toString() || ''}
                    onChange={(e) => {
                      if (e.target.value) {
                        actions.selectFramerate(parseInt(e.target.value, 10));
                      }
                    }}
                    className="block w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-800 dark:text-white"
                    disabled={isCreating || framerates.length === 0}
                  >
                    <option value="">Auto</option>
                    {framerates.map((rate) => {
                      const fpsValue = Math.round(rate.fps);
                      return (
                        <option key={`${rate.numerator}/${rate.denominator}`} value={fpsValue.toString()}>
                          {fpsValue} FPS ({rate.numerator}/{rate.denominator})
                        </option>
                      );
                    })}
                  </select>
                  );
                })()}
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
              <select
                value={state.bitrate?.toString() || ''}
                onChange={(e) => actions.setBitrate(e.target.value ? parseInt(e.target.value, 10) : undefined)}
                className="block w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-800 dark:text-white"
                disabled={isCreating}
              >
                {COMMON_BITRATES.map((bitrate) => (
                  <option key={bitrate.value} value={bitrate.value}>
                    {bitrate.label}
                  </option>
                ))}
              </select>
              {state.errors.bitrate && (
                <p className="mt-1 text-sm text-red-600 dark:text-red-400">{state.errors.bitrate}</p>
              )}
            </div>
          </div>
          
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