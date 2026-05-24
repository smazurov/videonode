import { Card } from '../Card';
import { SectionHeader } from '../primitives/SectionHeader';
import { KVInspector, type KVEntry } from '../primitives/KVInspector';
import { StatusPill } from '../primitives/StatusPill';
import type { SourceEntry } from '../../hooks/useSourceStore';

interface SourceStatusPanelProps {
  source: SourceEntry;
}

export function SourceStatusPanel({ source }: Readonly<SourceStatusPanelProps>) {
  const status = source.latest_status;

  const signalEntries: KVEntry[] = status
    ? [
        { key: 'signal_locked', value: String(status.signal.signal_locked) },
        { key: 'cable_present', value: String(status.signal.cable_present) },
        { key: 'has_dv_timings', value: String(status.signal.has_dv_timings) },
        { key: 'dv_timings', value: status.signal.dv_timings || '—' },
      ]
    : [];

  const broadcastEntries: KVEntry[] = status
    ? [
        { key: 'target_fps', value: String(status.broadcast.target_fps) },
        { key: 'last_seq', value: String(status.broadcast.last_seq) },
        { key: 'real_frames', value: String(status.broadcast.real_frames) },
        { key: 'placeholder_frames', value: String(status.broadcast.placeholder_frames) },
      ]
    : [];

  return (
    <Card padding="lg">
      <SectionHeader
        title="Status"
        description="Live status snapshot from the source sidecar."
        actions={<StatusPill status={source.status} />}
      />
      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
        <div>
          <h3 className="text-sm font-medium text-fg mb-2">Signal</h3>
          <KVInspector entries={signalEntries} emptyText="No signal data yet" />
        </div>
        <div>
          <h3 className="text-sm font-medium text-fg mb-2">Broadcast</h3>
          <KVInspector entries={broadcastEntries} emptyText="No broadcast data yet" />
        </div>
      </div>
      {source.last_status_at && (
        <p className="mt-4 text-xs text-fg-subtle">
          Last update {new Date(source.last_status_at).toLocaleString()}
        </p>
      )}
    </Card>
  );
}
