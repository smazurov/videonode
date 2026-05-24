import { Link } from 'react-router-dom';
import { useStreamStore } from '../../hooks/useStreamStore';
import { KVInspector, type KVEntry } from '../primitives/KVInspector';
import { SectionHeader } from '../primitives/SectionHeader';
import { cn } from '../../utils';

interface StreamEncoderPanelProps {
  readonly streamId: string;
  readonly className?: string;
}

export function StreamEncoderPanel({ streamId, className }: StreamEncoderPanelProps) {
  const stream = useStreamStore((state) => state.streamsById[streamId]);

  if (!stream) {
    return (
      <section className={cn('rounded-lg border border-border bg-surface-raised p-4', className)}>
        <p className="text-sm text-fg-muted">Stream not found.</p>
      </section>
    );
  }

  const encoder = stream.encoder ?? {};
  const codec = encoder.codec ?? '—';
  const bitrate = encoder.bitrate ?? '—';
  const gop = encoder.gop ?? '—';
  const rateControl = encoder.rate_control ?? '—';
  const preset = encoder.preset ?? '—';
  const customArgs = stream.custom_encoder_args ?? '';

  const entries: KVEntry[] = [
    { label: 'Codec', value: String(codec).toLowerCase(), mono: true },
    { label: 'Bitrate', value: String(bitrate), mono: true },
    { label: 'GOP', value: String(gop), mono: true },
    { label: 'Rate control', value: String(rateControl), mono: true },
    { label: 'Preset', value: String(preset), mono: true },
  ];

  return (
    <section className={cn('rounded-lg border border-border bg-surface-raised p-4', className)}>
      <SectionHeader
        title="Encoder"
        description="Read-only encoder configuration"
        actions={
          <Link
            to={`/streams/${encodeURIComponent(streamId)}/edit`}
            className="text-xs font-medium text-accent hover:underline"
          >
            Edit
          </Link>
        }
      />
      <div className="mt-3">
        <KVInspector entries={entries} dense />
        {customArgs && (
          <div className="mt-3 rounded-md border border-border bg-surface-muted/30 p-3">
            <div className="text-xs uppercase tracking-wide text-fg-muted">Custom args</div>
            <pre className="mt-1 whitespace-pre-wrap break-all font-mono text-xs text-fg">
              {customArgs}
            </pre>
          </div>
        )}
      </div>
    </section>
  );
}
