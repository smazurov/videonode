import { FormEvent } from 'react';
import { Card } from '../Card';
import { Button } from '../Button';
import { InputField } from '../InputField';
import { ReadOnlyField } from '../ReadOnlyField';
import { SourceDeviceField } from './SourceDeviceField';
import { SourceFormatSelector, type SourceFormatValue } from './SourceFormatSelector';
import { SourceTestModeToggle } from './SourceTestModeToggle';
import { useSourceForm } from '../../hooks/useSourceForm';
import type { SourceData } from '../../hooks/useSourceStore';
import type { components } from '../../lib/api.generated';

type SourceFormatBody = components['schemas']['SourceFormatBody'];

function formatToValue(f: SourceFormatBody | null): SourceFormatValue {
  if (!f) return { format_name: '', width: 0, height: 0, fps: 0 };
  return {
    format_name: f.format_name,
    width: f.width,
    height: f.height,
    fps: f.fps ?? 0,
  };
}

function valueToFormat(v: SourceFormatValue): SourceFormatBody | null {
  if (!v.format_name) return null;
  // Preserve partial selections (width/height/fps still 0 mid-cascade)
  // so the selector's auto-pick effects can fill them in. Returning
  // null when w/h are 0 would cause the format dropdown to read back
  // empty and snap to the first format every time the user picks a new
  // one. buildPatch / buildRequest filter out incomplete formats before
  // sending to the API.
  const out: SourceFormatBody = {
    format_name: v.format_name,
    width: v.width,
    height: v.height,
  };
  if (v.fps > 0) out.fps = v.fps;
  return out;
}

interface SourceFormProps {
  initialData?: SourceData;
  onSuccess: () => Promise<void>;
  onCancel?: () => void;
  className?: string;
}

export function SourceForm({
  initialData,
  onSuccess,
  onCancel,
  className = '',
}: Readonly<SourceFormProps>) {
  const form = useSourceForm(initialData);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    const ok = await form.submit();
    if (ok) await onSuccess();
  };

  const modeLabel = form.mode === 'edit' ? 'Save Changes' : 'Create Source';
  const submitLabel = form.saving ? 'Saving...' : modeLabel;

  const idErrorProps = form.errors.id ? { error: form.errors.id } : {};
  const deviceErrorProps = form.errors.device ? { error: form.errors.device } : {};

  return (
    <Card className={className}>
      <Card.Content>
        <form onSubmit={handleSubmit} className="space-y-6">
          {form.mode === 'edit' ? (
            <ReadOnlyField label="Source ID" value={form.id} mono />
          ) : (
            <InputField
              label="Source ID"
              type="text"
              value={form.id}
              onChange={(e) => form.setId(e.target.value)}
              placeholder="hdmi-slides"
              required
              disabled={form.saving}
              hint="Kebab-case: lowercase letters, digits, dashes. Must be unique."
              {...idErrorProps}
            />
          )}

          <SourceTestModeToggle
            value={form.testMode}
            onChange={form.toggleTestMode}
            disabled={form.saving}
          />

          {!form.testMode && (
            <>
              <SourceDeviceField
                value={form.device}
                onChange={form.setDevice}
                disabled={form.saving}
                required
                {...deviceErrorProps}
              />
              {form.device && (
                <SourceFormatSelector
                  deviceId={form.device}
                  value={formatToValue(form.format)}
                  onChange={(v) => form.setFormat(valueToFormat(v))}
                  disabled={form.saving}
                />
              )}
            </>
          )}

          {form.error && (
            <div className="p-3 border border-danger rounded-md bg-danger-soft">
              <p className="text-sm text-danger-soft-fg">{form.error}</p>
            </div>
          )}

          <div className="flex items-center justify-end gap-2">
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
              disabled={form.saving || !form.isValid || !form.isDirty}
              loading={form.saving}
              text={submitLabel}
            />
          </div>
        </form>
      </Card.Content>
    </Card>
  );
}
