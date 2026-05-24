import { InputField } from '../InputField';
import { ReadOnlyField } from '../ReadOnlyField';
import { UpstreamPicker } from './UpstreamPicker';

interface UpstreamSectionProps {
  mode: 'create' | 'edit';
  streamId: string;
  upstream: string;
  onStreamIdChange: (next: string) => void;
  onUpstreamChange: (next: string) => void;
  disabled?: boolean;
  errors: Record<string, string>;
}

export function UpstreamSection({
  mode,
  streamId,
  upstream,
  onStreamIdChange,
  onUpstreamChange,
  disabled,
  errors,
}: Readonly<UpstreamSectionProps>) {
  const streamIdErrorProps = errors.stream_id ? { error: errors.stream_id } : {};
  const upstreamErrorProps = errors.upstream ? { error: errors.upstream } : {};

  return (
    <section className="space-y-4">
      <h2 className="text-lg font-semibold text-fg">Identity & Upstream</h2>
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        {mode === 'edit' ? (
          <ReadOnlyField label="Stream ID" value={streamId} mono />
        ) : (
          <InputField
            label="Stream ID"
            type="text"
            value={streamId}
            onChange={(e) => onStreamIdChange(e.target.value)}
            placeholder="my-stream-001"
            required
            disabled={disabled}
            {...streamIdErrorProps}
          />
        )}
        <UpstreamPicker
          value={upstream}
          onChange={onUpstreamChange}
          disabled={disabled}
          required
          {...upstreamErrorProps}
        />
      </div>
    </section>
  );
}
