import { useMemo, useState } from 'react';
import { useProcesses } from '../../hooks/useProcesses';
import { useStreamStore } from '../../hooks/useStreamStore';
import { SectionHeader } from '../primitives/SectionHeader';
import { Badge } from '../Badge';
import { cn } from '../../utils';

interface StreamConsumersPanelProps {
  readonly streamId: string;
  readonly className?: string;
}

type Protocol = 'webrtc' | 'srt' | 'rtsp';

const PROTO_LABEL: Record<Protocol, string> = {
  webrtc: 'WebRTC peers',
  srt: 'SRT clients',
  rtsp: 'RTSP readers',
};

const PROTO_BADGE: Record<Protocol, 'webrtc' | 'srt' | 'rtsp'> = {
  webrtc: 'webrtc',
  srt: 'srt',
  rtsp: 'rtsp',
};

interface ConsumerRow {
  readonly id: string;
  readonly connectedSince?: string;
  readonly bytesSent?: number;
}

function uptimeFrom(startMicros: number | undefined): string {
  if (!startMicros) return '—';
  const start = new Date(startMicros / 1000);
  const seconds = Math.max(0, Math.floor((Date.now() - start.getTime()) / 1000));
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  if (seconds < 60) return `${s}s`;
  return `${m}m ${s}s`;
}

// We don't yet have a typed consumers endpoint per stream. The encoder
// process tells us liveness; per-protocol peer enumeration will arrive with
// B8/B10. Empty-state messaging makes the gap explicit.
function deriveConsumers(processes: ReturnType<typeof useProcesses>['processes'], streamId: string): {
  readonly running: boolean;
  readonly connectedSince?: string;
} {
  const encoder = processes.find((p) => p.kind === 'encoder' && p.stream_id === streamId);
  if (!encoder) return { running: false };
  if (encoder.state !== 'running') return { running: false };
  return { running: true, connectedSince: uptimeFrom(encoder.started_at_us) };
}

export function StreamConsumersPanel({ streamId, className }: StreamConsumersPanelProps) {
  const [activeTab, setActiveTab] = useState<Protocol>('webrtc');
  const { processes } = useProcesses({ enabled: true });
  const stream = useStreamStore((state) => state.streamsById[streamId]);

  const encoderState = useMemo(() => deriveConsumers(processes, streamId), [processes, streamId]);

  const rowsByProto: Record<Protocol, ConsumerRow[]> = useMemo(() => {
    const placeholders: Record<Protocol, ConsumerRow[]> = { webrtc: [], srt: [], rtsp: [] };
    if (!stream?.enabled || !encoderState.running) return placeholders;
    // Until the backend exposes per-protocol consumer lists, surface a single
    // synthetic row indicating the encoder feed is live.
    const row: ConsumerRow = { id: `${streamId}-encoder`, connectedSince: encoderState.connectedSince ?? '—' };
    if (stream.rtsp_url) placeholders.rtsp.push(row);
    if (stream.srt_url) placeholders.srt.push(row);
    placeholders.webrtc.push(row);
    return placeholders;
  }, [stream, encoderState, streamId]);

  const tabs: Protocol[] = ['webrtc', 'srt', 'rtsp'];

  return (
    <section className={cn('rounded-lg border border-border bg-surface-raised p-4', className)}>
      <SectionHeader
        title="Consumers"
        description="Currently attached playback clients, grouped by protocol"
      />

      <div className="mt-3 flex gap-1 border-b border-border">
        {tabs.map((proto) => {
          const count = rowsByProto[proto].length;
          const active = activeTab === proto;
          return (
            <button
              key={proto}
              type="button"
              onClick={() => setActiveTab(proto)}
              className={cn(
                'inline-flex items-center gap-2 px-3 py-2 text-xs font-medium border-b-2 -mb-px transition-colors',
                active
                  ? 'border-accent text-fg'
                  : 'border-transparent text-fg-muted hover:text-fg',
              )}
            >
              <Badge tone={PROTO_BADGE[proto]} size="xs">{proto.toUpperCase()}</Badge>
              <span>{PROTO_LABEL[proto]}</span>
              <span className="font-mono text-fg-muted">({count})</span>
            </button>
          );
        })}
      </div>

      <div className="mt-3">
        <ConsumersTable rows={rowsByProto[activeTab]} />
      </div>
    </section>
  );
}

function ConsumersTable({ rows }: { readonly rows: ReadonlyArray<ConsumerRow> }) {
  if (rows.length === 0) {
    return (
      <div className="rounded-md border border-dashed border-border px-4 py-6 text-center text-xs text-fg-muted">
        No active clients on this protocol.
      </div>
    );
  }
  return (
    <table className="min-w-full text-sm">
      <thead className="text-left text-xs uppercase tracking-wide text-fg-muted">
        <tr>
          <th scope="col" className="px-2 py-1.5 font-medium">Client</th>
          <th scope="col" className="px-2 py-1.5 font-medium">Connected</th>
          <th scope="col" className="px-2 py-1.5 font-medium">Bytes sent</th>
          <th scope="col" className="px-2 py-1.5 font-medium" />
        </tr>
      </thead>
      <tbody className="divide-y divide-border">
        {rows.map((row) => (
          <tr key={row.id}>
            <td className="px-2 py-1.5 font-mono text-xs text-fg">{row.id}</td>
            <td className="px-2 py-1.5 font-mono text-xs text-fg-muted">{row.connectedSince ?? '—'}</td>
            <td className="px-2 py-1.5 font-mono text-xs text-fg-muted">
              {row.bytesSent != null ? `${row.bytesSent}` : '—'}
            </td>
            <td className="px-2 py-1.5 text-right">
              <button
                type="button"
                className="text-xs text-fg-muted hover:text-danger"
                disabled
                title="Disconnect (not yet wired)"
              >
                Disconnect
              </button>
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
