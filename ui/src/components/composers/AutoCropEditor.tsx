import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import toast from 'react-hot-toast';
import { Button } from '../Button';
import { Select } from '../Select';
import { useSensorStore } from '../../hooks/useSensorStore';
import type { ComposerEffect } from '../../hooks/useComposerStore';

interface AutoCropEditorProps {
  inputRef: string;
  effect: ComposerEffect | null | undefined;
  saving: boolean;
  onSave: (effect: ComposerEffect | null) => Promise<void>;
  onCancel: () => void;
}

// AutoCropEditor binds a composer input to a first-class sensor. All detection
// + commit policy (detector, mode, margin, confidence) lives on the sensor
// itself, authored in the Sensors area — here the input just *picks* which
// sensor drives its crop, the way a stream picks its upstream.
export function AutoCropEditor({
  inputRef,
  effect,
  saving,
  onSave,
  onCancel,
}: Readonly<AutoCropEditorProps>) {
  const initialSensor = effect?.type === 'auto_crop' ? effect.auto_crop?.sensor ?? '' : '';
  const [sensorRef, setSensorRef] = useState<string>(initialSensor);
  const hadInitial = effect?.type === 'auto_crop';

  const sensorIds = useSensorStore((s) => s.sensorIds);
  const sensorsById = useSensorStore((s) => s.sensorsById);
  const lastUpdated = useSensorStore((s) => s.lastUpdated);
  const fetchSensors = useSensorStore((s) => s.fetchSensors);

  useEffect(() => {
    if (lastUpdated === null) void fetchSensors();
  }, [lastUpdated, fetchSensors]);

  const sensorRefs = useMemo(() => {
    const refs = sensorIds.map((id) => `sensor:${id}`);
    // Surface the current selection even if the sensor was since deleted.
    if (initialSensor && !refs.includes(initialSensor)) refs.push(initialSensor);
    return refs;
  }, [sensorIds, initialSensor]);

  const handleApply = useCallback(async () => {
    if (!sensorRef) {
      toast.error('Pick a sensor first');
      return;
    }
    try {
      await onSave({ type: 'auto_crop', auto_crop: { sensor: sensorRef } });
      toast.success(`Auto-crop on ${inputRef} → ${sensorRef}`);
    } catch (error) {
      toast.error('Failed to apply auto-crop');
      console.error(error);
    }
  }, [sensorRef, inputRef, onSave]);

  const handleRemove = useCallback(async () => {
    try {
      await onSave(null);
      toast.success(`Auto-crop cleared from ${inputRef}`);
    } catch (error) {
      toast.error('Failed to clear auto-crop');
      console.error(error);
    }
  }, [inputRef, onSave]);

  const selected = sensorRef ? sensorsById[sensorRef.replace(/^sensor:/, '')] : undefined;

  return (
    <div className="space-y-3">
      <p className="text-sm text-fg-subtle">
        Pick the sensor whose findings crop this input. The sensor carries its own detector
        and follow policy — edit those in the{' '}
        <Link to="/sensors" className="underline">
          Sensors
        </Link>{' '}
        area.
      </p>

      {sensorRefs.length === 0 ? (
        <p className="rounded-md border border-border bg-surface-muted p-3 text-sm text-fg-muted">
          No sensors yet.{' '}
          <Link to="/sensors/new" className="underline">
            Create a sensor
          </Link>{' '}
          first, then bind it here.
        </p>
      ) : (
        <Select
          label="Sensor"
          value={sensorRef}
          onChange={(e) => setSensorRef(e.target.value)}
          disabled={saving}
          hint={
            selected
              ? `Observes ${selected.source} · mode ${selected.mode ?? 'auto'}`
              : 'Select which sensor drives this input’s crop'
          }
        >
          <option value="">Select a sensor…</option>
          {sensorRefs.map((ref) => (
            <option key={ref} value={ref}>
              {ref}
            </option>
          ))}
        </Select>
      )}

      <div className="flex items-center gap-2">
        <Button
          theme="primary"
          size="SM"
          text={saving ? 'Applying...' : 'Apply'}
          onClick={handleApply}
          disabled={saving || !sensorRef}
        />
        {hadInitial && (
          <Button
            theme="danger"
            size="SM"
            text={saving ? 'Removing...' : 'Remove Auto-crop'}
            onClick={handleRemove}
            disabled={saving}
          />
        )}
        <Button theme="light" size="SM" text="Done" onClick={onCancel} disabled={saving} />
      </div>
    </div>
  );
}
