import type { ComponentProps } from 'react';
import { Card } from '../Card';
import { SectionHeader } from '../primitives/SectionHeader';
import { StatusPill, type StatusPillStatus } from '../primitives/StatusPill';
import { poolStateToPill } from '../../lib/pool-status';
import { ENCODED_FLOW, packetFlow, unitFlow, type StreamHealth } from '../../lib/stream-health';
import { secondsAgo, upstreamFramesLine, type Tone } from '../../lib/stream-status';
import type { MetricsSample } from '../../hooks/slices/streams/streamDataSlice';
import { MetricRow } from './MetricRow';
import { HOVER_HINT } from '../primitives/hoverHint';
import { cn } from '../../utils';
import { StreamPipelineDiagram } from './StreamPipelineDiagram';
import { useStreamPipeline } from './useStreamPipeline';
import { formatBitrate } from './format';

const SUMMARY_PILL: Record<Tone, StatusPillStatus> = { ok: 'running', warn: 'warm', bad: 'error', off: 'idle' };

const SUMMARY_TEXT: Record<Tone, string | undefined> = { ok: undefined, warn: 'text-warning', bad: 'text-danger', off: undefined };

function latestRate(history: readonly MetricsSample[], flow: string): number | null {
  return history[history.length - 1]?.rates[flow] ?? null;
}

function trackLine(track: StreamHealth['tracks'][number], history: readonly MetricsSample[]): string {
  if (track.packets.state === 'stalled') return `${track.type}: stopped ${secondsAgo(track.packets.age_ms)} ago`;
  const video = track.type === 'video';
  const pkts = latestRate(history, packetFlow(track.index));
  const units = latestRate(history, unitFlow(track.index));
  let out = '—';
  if (units != null) out = video ? `${units.toFixed(1)} fps` : `${Math.round(units)} fr/s`;
  const perFrame = video && track.units.count > 0 ? ` (${(track.packets.count / track.units.count).toFixed(1)} packets/frame)` : '';
  return `${track.type}: ${pkts == null ? '—' : Math.round(pkts)} pkt/s → ${out}${perFrame}`;
}

function encodedTooltip(health: StreamHealth | undefined, active: boolean): string {
  const stats = active ? health?.encoder.ffmpeg.stats : undefined;
  if (!stats) return 'Encoder not running';
  const lines = [`Speed ${stats.speed.toFixed(2)}×`, `ffmpeg: ${stats.dropped_frames} dropped · ${stats.duplicate_frames} duplicated`];
  const upstream = upstreamFramesLine(health);
  if (upstream) lines.push(upstream);
  return lines.join('\n');
}

function relayTooltip(health: StreamHealth | undefined, active: boolean, history: readonly MetricsSample[]): string | undefined {
  if (!active || !health || (health.tracks ?? []).length === 0) return undefined;
  return health.tracks.map((t) => trackLine(t, history)).join('\n');
}

function egressTooltip(watching: number, worstRtt: number | undefined): string {
  const rtt = worstRtt == null ? '' : ` · worst RTT ${worstRtt.toFixed(1)} ms`;
  return `${watching} watching${rtt}\nWebRTC and SRT viewer traffic`;
}

// One chart per stage: the headline number sits on the chart, the rest is in
// its tooltip.
function Chart(props: ComponentProps<typeof MetricRow>) {
  return (
    <div className="rounded-md border border-border bg-surface-muted/30">
      <MetricRow chartFraction={0.5} {...props} />
    </div>
  );
}

interface StreamStatusPanelProps {
  readonly streamId: string;
}

/** Health sentence, pipeline from source to viewers, and per-stage trends. */
export function StreamStatusPanel({ streamId }: StreamStatusPanelProps) {
  const p = useStreamPipeline(streamId);
  const worstRtt = p.viewers.reduce<number | undefined>((m, v) => (v.rtt && (m == null || v.rtt > m) ? v.rtt : m), undefined);
  const { tone, headline, detail } = p.summary;
  return (
    <Card padding="lg">
      <SectionHeader
        title="Status"
        description={
          tone === 'ok'
            ? undefined
            : <span className={SUMMARY_TEXT[tone]}>{headline}{detail ? ` — ${detail}` : ''}</span>
        }
        actions={
          <div className="flex items-center gap-2">
            <StatusPill status={poolStateToPill(p.encoderState)} />
            {tone !== 'off' && <StatusPill status={SUMMARY_PILL[tone]} label={tone === 'ok' ? 'Healthy' : 'Degraded'} />}
          </div>
        }
      />
      <div className="mb-4 mt-2 rounded-md border border-border bg-surface-muted/30 px-4 py-3">
        <StreamPipelineDiagram sources={p.sources} chain={p.chain} />
      </div>
      <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
        <Chart
          label="Encoded"
          values={p.history.map((h) => h.rates[ENCODED_FLOW] ?? null)}
          formatValue={(n) => `${n.toFixed(1)} fps`}
          tone="success"
          stale={!p.active}
          title={encodedTooltip(p.health, p.active)}
        />
        <Chart
          label="Relay lag"
          values={p.history.map((h) => h.relayMs)}
          formatValue={(n) => `${n < 10 ? n.toFixed(1) : Math.round(n)} ms`}
          tone="accent"
          stale={!p.active}
          title={relayTooltip(p.health, p.active, p.history)}
        />
        <Chart
          label="Egress"
          // Only samples taken while the encoder ran; flowCounts is empty otherwise.
          values={p.history.map((h) => (h.counts[ENCODED_FLOW] == null ? null : h.bitrateBps))}
          formatValue={formatBitrate}
          tone="accent"
          stale={!p.connected || !p.active}
          title={egressTooltip(p.viewers.length, worstRtt)}
        />
      </div>
      <div className="mt-4 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-fg-subtle">
        {p.facts && <span>{p.facts}</span>}
        {p.uptime && <span>Uptime <span className="text-fg">{p.uptime}</span></span>}
        {p.events.length > 0 && (
          <span className="flex flex-wrap items-center gap-x-3 gap-y-1">
            <span>Last 10s:</span>
            {p.events.map((e) => (
              <span key={e.key} title={e.title} className={cn(HOVER_HINT, e.tone === 'warn' ? 'text-warning' : 'text-fg-muted')}>
                {e.label}
              </span>
            ))}
          </span>
        )}
      </div>
    </Card>
  );
}
