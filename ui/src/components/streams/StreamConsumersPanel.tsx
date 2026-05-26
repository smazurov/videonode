import { useCallback, useMemo, useState } from 'react';
import { useProcesses } from '../../hooks/useProcesses';
import { useStreamStore } from '../../hooks/useStreamStore';
import { SectionHeader } from '../primitives/SectionHeader';
import { Badge } from '../Badge';
import { cn } from '../../utils';
import { API_BASE_URL } from '../../lib/api';
import { getAuthCredentials } from '../../lib/auth';

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
  readonly connectedSince?: string | undefined;
  readonly bytesSent?: number | undefined;
  readonly bitrate?: string | undefined;
  readonly qualityLabel?: string | undefined;
}

interface WebRTCClient {
  readonly id: string;
  readonly connected_since: string;
  readonly bytes_sent: number;
  readonly jitter_ms: number;
}

interface SRTClient {
  readonly id: string;
  readonly connected_since: string;
  readonly bytes_sent: number;
  readonly rtt_ms: number;
}

interface ConsumerCounts {
  total?: number;
  rtsp?: number;
  webrtc?: number;
  srt?: number;
  webrtc_clients?: WebRTCClient[];
  srt_clients?: SRTClient[];
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

function uptimeFromRFC3339(rfc3339: string): string {
  const start = new Date(rfc3339);
  if (isNaN(start.getTime())) return '—';
  const seconds = Math.max(0, Math.floor((Date.now() - start.getTime()) / 1000));
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  if (seconds < 60) return `${s}s`;
  const h = Math.floor(m / 60);
  if (h === 0) return `${m}m ${s}s`;
  return `${h}h ${m % 60}m`;
}

function formatRate(bytesPerSec: number): string {
  const bps = bytesPerSec * 8;
  if (bps < 1000) return `${Math.round(bps)} bps`;
  if (bps < 1_000_000) return `${(bps / 1000).toFixed(0)} Kbps`;
  return `${(bps / 1_000_000).toFixed(1)} Mbps`;
}

function deriveConsumers(processes: ReturnType<typeof useProcesses>['processes'], streamId: string): {
  readonly running: boolean;
  readonly connectedSince?: string;
} {
  const encoder = processes.find((p) => p.kind === 'encoder' && p.stream_id === streamId);
  if (!encoder) return { running: false };
  if (encoder.state !== 'running') return { running: false };
  return { running: true, connectedSince: uptimeFrom(encoder.started_at_us) };
}

type BytesSnapshot = Record<string, number>;

const bitrateCache = new Map<string, { prevBytes: BytesSnapshot; rates: Record<string, number> }>();

function computeBitrates(key: string, consumers: ConsumerCounts | undefined): Record<string, number> {
  let entry = bitrateCache.get(key);
  if (!entry) {
    entry = { prevBytes: {}, rates: {} };
    bitrateCache.set(key, entry);
  }

  const current: BytesSnapshot = {};
  for (const c of consumers?.webrtc_clients ?? []) current[c.id] = c.bytes_sent;
  for (const c of consumers?.srt_clients ?? []) current[c.id] = c.bytes_sent;

  const next: Record<string, number> = {};
  for (const [id, bytes] of Object.entries(current)) {
    const prev = entry.prevBytes[id];
    if (prev != null && bytes >= prev) {
      next[id] = bytes - prev;
    } else if (entry.rates[id] != null) {
      next[id] = entry.rates[id];
    }
  }

  entry.prevBytes = current;
  entry.rates = next;
  return next;
}

export function StreamConsumersPanel({ streamId, className }: StreamConsumersPanelProps) {
  const [activeTab, setActiveTab] = useState<Protocol>('webrtc');
  const { processes } = useProcesses({ enabled: true });
  const stream = useStreamStore((state) => state.streamsById[streamId]);
  const consumers = useStreamStore((state) => state.consumersById[streamId]) as ConsumerCounts | undefined;

  const encoderState = useMemo(() => deriveConsumers(processes, streamId), [processes, streamId]);
  const bitrates = useMemo(() => computeBitrates(streamId, consumers), [streamId, consumers]);

  const rowsByProto: Record<Protocol, ConsumerRow[]> = useMemo(() => {
    const result: Record<Protocol, ConsumerRow[]> = { webrtc: [], srt: [], rtsp: [] };
    if (!stream?.enabled || !encoderState.running) return result;
    if (!consumers) return result;

    const since = encoderState.connectedSince ?? '—';

    if (consumers.webrtc_clients?.length) {
      result.webrtc = consumers.webrtc_clients.map((c) => ({
        id: c.id,
        connectedSince: c.connected_since,
        bitrate: c.id in bitrates ? formatRate(bitrates[c.id] ?? 0) : undefined,
        qualityLabel: c.jitter_ms > 0 ? `${c.jitter_ms.toFixed(1)} ms` : undefined,
      }));
    } else {
      for (let i = 0; i < (consumers.webrtc ?? 0); i++) {
        result.webrtc.push({ id: `${streamId}-webrtc-${i}`, connectedSince: since });
      }
    }

    if (consumers.srt_clients?.length) {
      result.srt = consumers.srt_clients.map((c) => ({
        id: c.id,
        connectedSince: c.connected_since,
        bitrate: c.id in bitrates ? formatRate(bitrates[c.id] ?? 0) : undefined,
        qualityLabel: c.rtt_ms > 0 ? `${c.rtt_ms.toFixed(1)} ms` : undefined,
      }));
    } else {
      for (let i = 0; i < (consumers.srt ?? 0); i++) {
        result.srt.push({ id: `${streamId}-srt-${i}`, connectedSince: since });
      }
    }

    for (let i = 0; i < (consumers.rtsp ?? 0); i++) {
      result.rtsp.push({ id: `${streamId}-rtsp-${i}`, connectedSince: since });
    }

    return result;
  }, [stream, encoderState, streamId, consumers, bitrates]);

  const tabs: Protocol[] = ['webrtc', 'srt', 'rtsp'];
  const qualityHeaders: Partial<Record<Protocol, string>> = { webrtc: 'Jitter', srt: 'RTT' };
  const qualityHeader = qualityHeaders[activeTab];

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
        <ConsumersTable
          rows={rowsByProto[activeTab]}
          qualityHeader={qualityHeader}
          streamId={streamId}
          protocol={activeTab}
        />
      </div>
    </section>
  );
}

function disconnectConsumer(streamId: string, protocol: Protocol, clientId: string) {
  const credentials = getAuthCredentials();
  fetch(`${API_BASE_URL}/api/streams/${encodeURIComponent(streamId)}/${protocol}/consumers/${encodeURIComponent(clientId)}`, {
    method: 'DELETE',
    headers: credentials ? { Authorization: `Basic ${credentials}` } : {},
  });
}

function ConsumersTable({ rows, qualityHeader, streamId, protocol }: {
  readonly rows: ReadonlyArray<ConsumerRow>;
  readonly qualityHeader?: string | undefined;
  readonly streamId: string;
  readonly protocol: Protocol;
}) {
  const canDisconnect = protocol === 'webrtc' || protocol === 'srt';

  const onDisconnect = useCallback((clientId: string) => {
    disconnectConsumer(streamId, protocol, clientId);
  }, [streamId, protocol]);

  if (rows.length === 0) {
    return (
      <div className="rounded-md border border-dashed border-border px-4 py-6 text-center text-xs text-fg-muted">
        No active clients on this protocol.
      </div>
    );
  }

  return (
    <table className="w-full table-fixed text-sm">
      <thead className="text-left text-xs uppercase tracking-wide text-fg-muted">
        <tr>
          <th scope="col" className="w-[30%] px-2 py-1.5 font-medium">Client</th>
          <th scope="col" className="w-[15%] px-2 py-1.5 font-medium">Connected</th>
          <th scope="col" className="w-[15%] px-2 py-1.5 font-medium">Rate</th>
          {qualityHeader && <th scope="col" className="w-[15%] px-2 py-1.5 font-medium">{qualityHeader}</th>}
          {canDisconnect && <th scope="col" className="w-[25%] px-2 py-1.5 font-medium" />}
        </tr>
      </thead>
      <tbody className="divide-y divide-border">
        {rows.map((row) => (
          <tr key={row.id}>
            <td className="w-[30%] truncate px-2 py-1.5 font-mono text-xs text-fg">{row.id}</td>
            <td className="w-[15%] px-2 py-1.5 font-mono text-xs text-fg-muted">
              {row.connectedSince ? uptimeFromRFC3339(row.connectedSince) : '—'}
            </td>
            <td className="w-[15%] px-2 py-1.5 font-mono text-xs text-fg-muted">
              {row.bitrate ?? '—'}
            </td>
            {qualityHeader && (
              <td className="w-[15%] px-2 py-1.5 font-mono text-xs text-fg-muted">
                {row.qualityLabel ?? '—'}
              </td>
            )}
            {canDisconnect && (
              <td className="w-[25%] px-2 py-1.5 text-right">
                <button
                  type="button"
                  className="text-xs text-fg-muted hover:text-danger transition-colors"
                  onClick={() => onDisconnect(row.id)}
                >
                  Disconnect
                </button>
              </td>
            )}
          </tr>
        ))}
      </tbody>
    </table>
  );
}
