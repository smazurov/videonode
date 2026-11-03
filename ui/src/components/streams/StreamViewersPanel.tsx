import { XMarkIcon } from '@heroicons/react/24/outline';
import { Card } from '../Card';
import { Badge } from '../Badge';
import { SectionHeader } from '../primitives/SectionHeader';
import { EmptyState } from '../primitives/EmptyState';
import { apiStreams } from '../../lib/api';
import { formatUptime } from '../../lib/formatUptime';
import { formatBitrate } from './format';
import { useStreamViewers, type StreamViewer } from './useStreamViewers';

const CELL = 'px-2 py-2.5 font-mono text-xs text-fg-muted';
const HEAD = 'px-2 py-2 font-medium';

function ms(value: number | undefined): string {
  return value ? `${value.toFixed(1)} ms` : '—';
}

function connectedFor(since: string | undefined): string {
  return since ? (formatUptime(Date.parse(since) * 1000) ?? '—') : '—';
}

function disconnect(streamId: string, viewer: StreamViewer) {
  if (viewer.protocol === 'rtsp') return;
  apiStreams
    .DELETE('/api/streams/{stream_id}/{protocol}/consumers/{client_id}', {
      params: { path: { stream_id: streamId, protocol: viewer.protocol, client_id: viewer.id } },
    })
    .catch((error: unknown) => {
      console.error(`Failed to disconnect ${viewer.protocol} viewer ${viewer.id}:`, error);
    });
}

interface StreamViewersPanelProps {
  readonly streamId: string;
}

/** One table for every network viewer; secondary columns drop on small screens. */
export function StreamViewersPanel({ streamId }: StreamViewersPanelProps) {
  const viewers = useStreamViewers(streamId);
  return (
    <Card padding="lg">
      <SectionHeader title="Viewers" />
      {viewers.length === 0 ? (
        <EmptyState title="No viewers" description="Open the player or connect over RTSP/SRT to start the encoder." />
      ) : (
        <div className="mt-3 overflow-x-auto">
          <table className="w-full border-collapse text-sm sm:table-fixed">
            <thead className="border-b border-border-strong bg-surface-muted/40 text-left text-xs uppercase tracking-wide text-fg-muted">
              <tr>
                <th scope="col" className={`w-[10%] ${HEAD}`}>Protocol</th>
                <th scope="col" className={`w-[18%] ${HEAD}`}>Client</th>
                <th scope="col" className={`hidden w-[18%] md:table-cell ${HEAD}`}>Client IP</th>
                <th scope="col" className={`hidden w-[12%] sm:table-cell ${HEAD}`}>Connected</th>
                <th scope="col" className={`w-[12%] ${HEAD}`}>Rate</th>
                <th scope="col" className={`w-[10%] ${HEAD}`}>RTT</th>
                <th scope="col" className={`hidden w-[10%] md:table-cell ${HEAD}`}>Jitter</th>
                <th scope="col" className={`w-[10%] ${HEAD}`}><span className="sr-only">Actions</span></th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {viewers.map((v) => (
                <tr key={v.key} className="transition-colors hover:bg-surface-muted/40">
                  <td className="px-2 py-2.5"><Badge tone={v.protocol} size="xs">{v.protocol}</Badge></td>
                  <td className="truncate px-2 py-2.5 font-mono text-xs text-fg">{v.id}</td>
                  <td className={`hidden truncate md:table-cell ${CELL}`}>{v.ip ?? '—'}</td>
                  <td className={`hidden sm:table-cell ${CELL}`}>{connectedFor(v.since)}</td>
                  <td className={CELL}>{v.rate == null ? '—' : formatBitrate(v.rate)}</td>
                  <td className={CELL}>{ms(v.rtt)}</td>
                  <td className={`hidden md:table-cell ${CELL}`}>{ms(v.jitter)}</td>
                  <td className="px-2 py-2.5 text-right">
                    {v.protocol !== 'rtsp' && (
                      <button
                        type="button"
                        onClick={() => disconnect(streamId, v)}
                        title="Disconnect"
                        aria-label={`Disconnect ${v.id}`}
                        className="inline-flex cursor-pointer items-center rounded-sm text-xs font-medium text-accent transition-colors hover:text-danger focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 focus-visible:ring-offset-surface-raised"
                      >
                        <span className="hidden sm:inline">Disconnect</span>
                        <XMarkIcon className="h-4 w-4 sm:hidden" />
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </Card>
  );
}
