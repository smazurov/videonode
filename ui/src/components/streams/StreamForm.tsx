import { FormEvent } from 'react';
import { Card } from '../Card';
import { Button } from '../Button';
import { useStreamForm } from '../../hooks/useStreamForm';
import type { components } from '../../lib/api.generated';
import { UpstreamSection } from './UpstreamSection';
import { EncoderFields } from './EncoderFields';
import { AudioFields } from './AudioFields';

type StreamData = components['schemas']['StreamData'];

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

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    const ok = await form.submit();
    if (ok) await onSuccess();
  };

  const submitLabel = (() => {
    if (form.saving) return 'Saving...';
    return form.mode === 'edit' ? 'Save Changes' : 'Create Stream';
  })();

  return (
    <Card className={className}>
      <Card.Content>
        <form onSubmit={handleSubmit} className="space-y-8">
          <UpstreamSection
            mode={form.mode}
            streamId={form.value.stream_id}
            upstream={form.value.upstream}
            onStreamIdChange={form.setStreamId}
            onUpstreamChange={form.setUpstream}
            disabled={form.saving}
            errors={form.errors}
          />

          <EncoderFields
            value={form.value.encoder}
            customEncoderArgs={form.value.custom_encoder_args ?? ''}
            onChange={form.setEncoder}
            onCustomEncoderArgsChange={form.setCustomEncoderArgs}
            disabled={form.saving}
            errors={form.errors}
          />

          <AudioFields
            value={form.value.audio}
            onChange={form.setAudio}
            disabled={form.saving}
            errors={form.errors}
          />

          {form.error && (
            <div className="rounded-md border border-danger/40 bg-danger/10 px-3 py-2 text-sm text-danger-soft-fg">
              {form.error}
            </div>
          )}

          <div className="flex items-center justify-end gap-3 pt-2 border-t border-border">
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
              disabled={!form.isValid || !form.isDirty || form.saving}
              loading={form.saving}
              text={submitLabel}
            />
          </div>
        </form>
      </Card.Content>
    </Card>
  );
}
