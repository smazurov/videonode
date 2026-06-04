import { useState, useCallback } from 'react';
import toast from 'react-hot-toast';

import { Button } from '../Button';
import { Card } from '../Card';
import { InputField } from '../InputField';
import { Select } from '../Select';
import { ReadOnlyField } from '../ReadOnlyField';
import { UpstreamPicker } from '../streams/UpstreamPicker';
import { useSensorStore } from '../../hooks/useSensorStore';
import type { SensorData } from '../../hooks/slices/types';

interface SensorFormProps {
  initialData?: SensorData;
  onSuccess: () => Promise<void> | void;
  onCancel?: () => void;
}

const KEBAB = /^[\da-z][\da-z-]*$/;

// SensorForm creates or edits a first-class sensor. It carries the full
// detection + commit policy (detector, mode, margin, confidence floor, tick) —
// composer inputs only *pick* a sensor, so this is the single place the policy
// is authored. The detector field is the swappable Python/native runtime.
export function SensorForm({ initialData, onSuccess, onCancel }: Readonly<SensorFormProps>) {
  const isEdit = !!initialData;
  const createSensor = useSensorStore((s) => s.createSensor);
  const updateSensor = useSensorStore((s) => s.updateSensor);

  const [id, setId] = useState(initialData?.id ?? '');
  const [source, setSource] = useState(initialData?.source ?? '');
  const [detector, setDetector] = useState(initialData?.detector ?? '');
  const [modelId, setModelId] = useState(initialData?.model_id ?? '');
  const [mode, setMode] = useState<'auto' | 'propose'>(
    (initialData?.mode as 'auto' | 'propose' | undefined) ?? 'auto',
  );
  const [margin, setMargin] = useState<number>(initialData?.margin ?? 0.1);
  const [minConfidence, setMinConfidence] = useState<number>(initialData?.min_confidence ?? 0.8);
  const [tickMs, setTickMs] = useState<number>(initialData?.tick_ms ?? 0);
  const [saving, setSaving] = useState(false);

  const idError = !isEdit && id !== '' && !KEBAB.test(id) ? 'Must be kebab-case' : undefined;
  let sourceError: string | undefined;
  if (source !== '' && !/^(source|composer):.+/.test(source)) {
    sourceError = 'Must be source:<id> or composer:<id>';
  }
  const canSubmit = (isEdit || (id !== '' && !idError)) && source !== '' && !sourceError && !saving;
  const idErrorProps = idError ? { error: idError } : {};
  const sourceErrorProps = sourceError ? { error: sourceError } : {};

  let submitLabel = 'Create sensor';
  if (saving) submitLabel = 'Saving…';
  else if (isEdit) submitLabel = 'Save changes';

  const handleSubmit = useCallback(async () => {
    setSaving(true);
    try {
      if (isEdit && initialData?.id) {
        await updateSensor(initialData.id, {
          source,
          detector,
          model_id: modelId,
          mode,
          margin,
          min_confidence: minConfidence,
          tick_ms: tickMs,
        });
        toast.success(`Sensor ${initialData.id} updated`);
      } else {
        await createSensor({
          id,
          source,
          ...(detector.trim() ? { detector: detector.trim() } : {}),
          ...(modelId.trim() ? { model_id: modelId.trim() } : {}),
          mode,
          margin,
          min_confidence: minConfidence,
          ...(tickMs > 0 ? { tick_ms: tickMs } : {}),
        });
        toast.success(`Sensor ${id} created`);
      }
      await onSuccess();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Failed to save sensor');
    } finally {
      setSaving(false);
    }
  }, [
    isEdit, initialData, id, source, detector, modelId, mode, margin, minConfidence, tickMs,
    createSensor, updateSensor, onSuccess,
  ]);

  return (
    <Card padding="lg" className="space-y-5">
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
        {isEdit ? (
          <ReadOnlyField label="Sensor ID" value={id} mono />
        ) : (
          <InputField
            label="Sensor ID"
            type="text"
            value={id}
            onChange={(e) => setId(e.target.value)}
            placeholder="playfield"
            required
            disabled={saving}
            {...idErrorProps}
          />
        )}
        <UpstreamPicker
          value={source}
          onChange={setSource}
          disabled={saving}
          required
          {...sourceErrorProps}
        />
      </div>

      <p className="text-sm text-fg-subtle">
        The sensor runs a detector on a small analysis tap of its observed ref and emits
        findings whether or not anything is bound to it. <strong>Auto</strong> applies the
        crop live within guardrails; <strong>Propose</strong> surfaces a candidate for you
        to confirm.
      </p>

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <Select
          label="Mode"
          value={mode}
          onChange={(e) => setMode(e.target.value as 'auto' | 'propose')}
          disabled={saving}
        >
          <option value="auto">Auto (apply)</option>
          <option value="propose">Propose (confirm)</option>
        </Select>
        <InputField
          label="Margin"
          type="number"
          step="0.05"
          min="0"
          max="0.5"
          value={margin}
          onChange={(e) => setMargin(Number(e.target.value))}
          hint="Bleed around the region (0.1 = 10%)"
          disabled={saving}
        />
        <InputField
          label="Min confidence"
          type="number"
          step="0.05"
          min="0"
          max="1"
          value={minConfidence}
          onChange={(e) => setMinConfidence(Number(e.target.value))}
          hint="Below this the crop holds, then widens"
          disabled={saving}
        />
      </div>

      <InputField
        label="Detector command"
        type="text"
        value={detector}
        onChange={(e) => setDetector(e.target.value)}
        placeholder="uv run sensors/playfield/detect.py"
        hint="The detector runtime (Python/native). Blank uses the daemon default."
        disabled={saving}
        className="font-mono"
      />

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <InputField
          label="Model ID"
          type="text"
          value={modelId}
          onChange={(e) => setModelId(e.target.value)}
          placeholder="playfield-classical-v0"
          hint="Tags emitted findings. Blank uses the daemon default."
          disabled={saving}
        />
        <InputField
          label="Re-detect interval (ms)"
          type="number"
          step="50"
          min="0"
          value={tickMs}
          onChange={(e) => setTickMs(Number(e.target.value))}
          hint="0 = the sensor binary's default cadence"
          disabled={saving}
        />
      </div>

      <div className="flex items-center gap-2">
        <Button
          theme="primary"
          size="MD"
          text={submitLabel}
          onClick={handleSubmit}
          disabled={!canSubmit}
        />
        {onCancel && (
          <Button theme="light" size="MD" text="Cancel" onClick={onCancel} disabled={saving} />
        )}
      </div>
    </Card>
  );
}
