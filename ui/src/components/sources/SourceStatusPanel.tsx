import { Card } from '../Card';
import { SectionHeader } from '../primitives/SectionHeader';
import { KVInspector, type KVEntry } from '../primitives/KVInspector';
import { StatusPill } from '../primitives/StatusPill';
import { poolStateToPill, livenessToPill } from '../../lib/pool-status';
import type { SourceEntry } from '../../hooks/useSourceStore';
import { formatUptime } from '../../lib/formatUptime';

interface SourceStatusPanelProps {
  source: SourceEntry;
}

function colorimetryLabel(matrix: string | undefined): string {
  if (matrix === 'bt709') return 'BT.709';
  if (matrix === 'bt601') return 'BT.601';
  return '—';
}

export function SourceStatusPanel({ source }: Readonly<SourceStatusPanelProps>) {
  const status = source.latest_status;

  const formatEntries: KVEntry[] = status
    ? [
        { label: 'pixel_format', value: status.format.fourcc || '—' },
        { label: 'mode', value: status.format.mode || '—' },
        { label: 'colorimetry', value: colorimetryLabel(status.format.color_matrix) },
      ]
    : [];

  const signalEntries: KVEntry[] = status
    ? [
        { label: 'signal_locked', value: String(status.signal.signal_locked) },
        { label: 'cable_present', value: String(status.signal.cable_present) },
        { label: 'has_dv_timings', value: String(status.signal.has_dv_timings) },
        { label: 'dv_timings', value: status.signal.dv_timings || '—' },
      ]
    : [];

  const broadcastEntries: KVEntry[] = status
    ? [
        {
          label: 'effective_fps',
          value: source.effective_fps !== undefined ? source.effective_fps.toFixed(1) : '—',
        },
        { label: 'last_seq', value: String(status.broadcast.last_seq) },
        { label: 'real_frames', value: String(status.broadcast.real_frames) },
        { label: 'placeholder_frames', value: String(status.broadcast.placeholder_frames) },
      ]
    : [];
  if (status && poolStateToPill(source.status) !== 'running') {
    broadcastEntries.push({
      label: 'placeholder_cadence',
      value: `${status.broadcast.target_fps} fps`,
    });
  }

  const lastUpdateMs = source.last_status_at
    ? new Date(source.last_status_at).getTime()
    : undefined;
  const uptimeLabel = formatUptime(source.started_at_us, lastUpdateMs);
  const liveness = livenessToPill(source.liveness ?? source.latest_status?.health);

  return (
    <Card padding="lg">
      <SectionHeader
        title="Status"
        description="Live status snapshot from the source sidecar."
        actions={
          <div className="flex items-center gap-2">
            <StatusPill status={poolStateToPill(source.status)} />
            <StatusPill status={liveness.status} label={liveness.label} />
          </div>
        }
      />
      <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
        <div>
          <h3 className="text-sm font-medium text-fg mb-2">Format</h3>
          <KVInspector entries={formatEntries} emptyText="No format data yet" />
        </div>
        <div>
          <h3 className="text-sm font-medium text-fg mb-2">Signal</h3>
          <KVInspector entries={signalEntries} emptyText="No signal data yet" />
        </div>
        <div>
          <h3 className="text-sm font-medium text-fg mb-2">Broadcast</h3>
          <KVInspector entries={broadcastEntries} emptyText="No broadcast data yet" />
        </div>
      </div>
      <div className="mt-4 flex flex-wrap gap-x-4 gap-y-1 text-xs text-fg-subtle">
        {source.consumer_count !== undefined && (
          <span>
            Consumers <span className="text-fg">{source.consumer_count}</span>
          </span>
        )}
        {uptimeLabel && (
          <span>
            Uptime <span className="text-fg">{uptimeLabel}</span>
          </span>
        )}
        {source.last_status_at && (
          <span>Last update {new Date(source.last_status_at).toLocaleString()}</span>
        )}
      </div>
    </Card>
  );
}
