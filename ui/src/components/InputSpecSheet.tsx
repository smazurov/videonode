import { useCallback, useState } from 'react';
import toast from 'react-hot-toast';
import { Button } from './Button';
import { BottomSheet } from './BottomSheet';
import { DeviceInputCapabilities } from './DeviceInputCapabilities';
import { api } from '../lib/api';
import { parseResolution } from '../lib/resolution';
import { useStreamStore } from '../hooks/useStreamStore';
import { useDeviceInputForm } from '../hooks/useDeviceInputForm';
import type { components } from '../lib/api.generated';

type StreamUpdateBody = components['schemas']['StreamUpdateRequestData'];

interface InputSpecSheetProps {
  isOpen: boolean;
  onClose: () => void;
  streamId: string;
}

const selectClasses =
  'block w-full px-3 py-2 border border-border rounded-md shadow-sm bg-surface text-fg focus:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:border-accent';

export function InputSpecSheet({ isOpen, onClose, streamId }: Readonly<InputSpecSheetProps>) {
  return (
    <BottomSheet
      open={isOpen}
      onClose={onClose}
      title={`Edit Input — ${streamId}`}
      maxWidth="2xl"
    >
      {/* Body mounted only when open so device-capability queries
          don't fire for every owned source on the streams page */}
      {isOpen && <SheetBody streamId={streamId} onClose={onClose} />}
    </BottomSheet>
  );
}

interface SheetBodyProps {
  streamId: string;
  onClose: () => void;
}

function SheetBody({ streamId, onClose }: Readonly<SheetBodyProps>) {
  const stream = useStreamStore((state) => state.streamsById[streamId]);
  const initialRotation = stream?.rotation ?? 0;
  const [rotation, setRotation] = useState<number>(initialRotation);
  const [saving, setSaving] = useState(false);

  const initialFramerate = stream?.framerate ? Number.parseInt(stream.framerate, 10) || 0 : 0;
  const { width: initW, height: initH } = parseResolution(stream?.resolution);
  const inputForm = useDeviceInputForm({
    deviceId: stream?.device_id ?? '',
    initial: stream
      ? {
          inputFormat: stream.input_format ?? '',
          width: initW,
          height: initH,
          framerate: initialFramerate,
        }
      : undefined,
    autoSelect: false,
  });

  const rotationDirty = rotation !== initialRotation;
  const isDirty = inputForm.isDirty || rotationDirty;

  const handleSave = useCallback(async () => {
    if (!isDirty || saving || !stream) return;
    const inputDiff = inputForm.diff();
    const body: StreamUpdateBody = {};
    if (inputDiff.input_format !== undefined) body.input_format = inputDiff.input_format;
    if (inputDiff.width !== undefined) body.width = inputDiff.width;
    if (inputDiff.height !== undefined) body.height = inputDiff.height;
    if (inputDiff.framerate !== undefined) body.framerate = inputDiff.framerate;
    if (rotationDirty) body.rotation = rotation as 0 | 90 | 180 | 270;
    setSaving(true);
    try {
      const { error } = await api.PATCH('/api/streams/{stream_id}', {
        params: { path: { stream_id: streamId } },
        body,
      });
      if (error) throw new Error(error.detail ?? 'Failed to save');
      toast.success('Input updated');
      onClose();
    } catch (error) {
      console.error('Failed to save input spec:', error);
      toast.error(error instanceof Error ? error.message : 'Failed to save');
    } finally {
      setSaving(false);
    }
  }, [isDirty, saving, stream, inputForm, rotationDirty, rotation, streamId, onClose]);

  if (!stream) return null;

  return (
    <>
      <p className="text-xs text-fg-subtle mb-4">
        Capture parameters sent to the device. The owning canvas will restart to
        pick up the changes.
      </p>

      <div className="space-y-4">
        <DeviceInputCapabilities form={inputForm} mode="edit" disabled={saving} />

        <div>
          <label className="block text-sm font-medium text-fg mb-2">Rotation</label>
          <select
            value={rotation}
            onChange={(e) => setRotation(Number.parseInt(e.target.value, 10))}
            className={selectClasses}
            disabled={saving}
          >
            <option value={0}>0°</option>
            <option value={90}>90° CW</option>
            <option value={180}>180°</option>
            <option value={270}>270° CW</option>
          </select>
        </div>
      </div>

      <div className="flex items-center justify-end gap-2 mt-6">
        <Button theme="light" size="SM" text="Cancel" onClick={onClose} disabled={saving} />
        <Button
          theme="primary"
          size="SM"
          text={saving ? 'Saving...' : 'Save'}
          onClick={handleSave}
          disabled={saving || !isDirty}
        />
      </div>
    </>
  );
}
