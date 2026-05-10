import { FormEvent } from 'react';
import { Card } from '../Card';
import { Button } from '../Button';
import { InputField } from '../InputField';
import { Select } from '../Select';
import { ReadOnlyField } from '../ReadOnlyField';
import { useDeviceStore } from '../../hooks/useDeviceStore';
import { useStreamForm } from '../../hooks/useStreamForm';
import { AdvancedOptions } from '../StreamCreation/AdvancedOptions';
import { AudioDeviceSelector } from '../StreamCreation/AudioDeviceSelector';
import { DeviceInputCapabilities } from '../DeviceInputCapabilities';
import type { components } from '../../lib/api.generated';

type StreamData = components["schemas"]["StreamData"];

interface StreamFormProps {
  initialData?: StreamData;
  onSuccess: () => Promise<void>;
  onCancel?: () => void;
  className?: string;
}

export function StreamForm({
  initialData,
  onSuccess,
  onCancel,
  className = '',
}: Readonly<StreamFormProps>) {
  const form = useStreamForm(initialData);
  const devices = useDeviceStore((s) => s.devices);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    const success = await form.submit();
    if (success) {
      await onSuccess();
    }
  };

  const deviceDisplayName = (() => {
    const device = devices.find((d) => d.device_id === form.deviceId);
    return device ? `${device.device_name} (${device.device_path})` : form.deviceId;
  })();

  const modeLabel = form.mode === 'edit' ? 'Save Changes' : 'Create Stream';
  const submitLabel = form.saving ? 'Saving...' : modeLabel;

  return (
    <Card className={className}>
      {form.mode === 'create' && (
        <Card.Header>
          <h3 className="text-lg font-semibold text-fg">Create New Stream</h3>
          <p className="text-sm text-fg-muted mt-1">
            Configure a new video stream from your capture devices
          </p>
        </Card.Header>
      )}

      <Card.Content>
        <form onSubmit={handleSubmit} className="space-y-6">
          {/* Stream ID */}
          {form.mode === 'edit' && (
            <ReadOnlyField label="Stream ID" value={form.streamId} mono />
          )}
          {form.mode === 'create' && (
            <InputField
              label="Stream ID"
              type="text"
              value={form.streamId}
              onChange={(e) => form.setStreamId(e.target.value)}
              placeholder="my-stream-001"
              required
              disabled={form.saving}
              {...(form.errors.streamId ? { error: form.errors.streamId } : {})}
            />
          )}

          {/* Device Selection - Video and Audio */}
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            {form.mode === 'edit' && (
              <ReadOnlyField label="Video Device" value={deviceDisplayName} />
            )}
            {form.mode === 'create' && (
              <Select
                label="Video Device"
                required
                value={form.deviceId}
                onChange={(e) => form.selectDevice(e.target.value)}
                disabled={form.saving || devices.length === 0}
                {...(form.errors.deviceId ? { error: form.errors.deviceId } : {})}
              >
                <option value="">Select device...</option>
                {devices.map((device) => (
                  <option key={device.device_id} value={device.device_id}>
                    {device.device_name} ({device.device_path})
                  </option>
                ))}
              </Select>
            )}

            <AudioDeviceSelector
              value={form.audioDevice}
              onChange={form.setAudioDevice}
              disabled={form.saving}
            />
          </div>

          {/* Format / Resolution / Framerate */}
          {(form.mode === 'edit' || form.deviceId) && (
            <DeviceInputCapabilities
              form={form.inputForm}
              mode={form.mode}
              disabled={form.saving}
              errors={form.errors}
            />
          )}

          {/* Codec, Bitrate, Rotation */}
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            <Select
              label="Codec"
              required
              value={form.codec}
              onChange={(e) => form.setCodec(e.target.value)}
              disabled={form.saving}
              {...(form.errors.codec ? { error: form.errors.codec } : {})}
            >
              <option value="h264">H.264</option>
              <option value="h265">H.265 (HEVC)</option>
            </Select>

            <div>
              <label className="block text-sm font-medium text-fg mb-2">Bitrate</label>
              <div className="relative">
                <input
                  type="number"
                  value={form.bitrate || 2}
                  onChange={(e) => {
                    const mbps = parseFloat(e.target.value);
                    if (!isNaN(mbps) && mbps > 0) {
                      form.setBitrate(mbps);
                    } else if (e.target.value === '') {
                      form.setBitrate(2);
                    }
                  }}
                  placeholder="2.0"
                  step="0.1"
                  min="0.1"
                  max="50"
                  className="block w-full pl-3 pr-16 py-2 border border-border rounded-md shadow-sm bg-surface text-fg focus:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:border-accent"
                  disabled={form.saving}
                />
                <div className="absolute inset-y-0 right-0 pr-3 flex items-center pointer-events-none">
                  <span className="text-fg-subtle sm:text-sm">Mbps</span>
                </div>
              </div>
              {form.errors.bitrate && (
                <p className="mt-1 text-sm text-danger-soft-fg">{form.errors.bitrate}</p>
              )}
            </div>

            <Select
              label="Rotation"
              value={form.rotation}
              onChange={(e) => form.setRotation(parseInt(e.target.value, 10))}
              disabled={form.saving}
            >
              <option value={0}>0°</option>
              <option value={90}>90° CW</option>
              <option value={180}>180°</option>
              <option value={270}>270° CW</option>
            </Select>
          </div>

          {/* Advanced Options */}
          <AdvancedOptions
            selectedOptions={form.options}
            onOptionsChange={form.setOptions}
            disabled={form.saving}
            className="mt-4"
          />

          {/* Error message */}
          {form.error && (
            <div className="p-3 border border-danger rounded-md bg-danger-soft">
              <p className="text-sm text-danger-soft-fg">{form.error}</p>
            </div>
          )}

          {/* Action Buttons */}
          <div className="flex justify-end space-x-3 pt-4 border-t border-border">
            {onCancel && (
              <Button
                type="button"
                theme="light"
                size="MD"
                onClick={onCancel}
                disabled={form.saving}
                text="Cancel"
              />
            )}

            <Button
              type="submit"
              theme="primary"
              size="MD"
              disabled={form.saving || !form.isValid || (form.mode === 'edit' && !form.isDirty)}
              text={submitLabel}
            />
          </div>
        </form>
      </Card.Content>
    </Card>
  );
}
