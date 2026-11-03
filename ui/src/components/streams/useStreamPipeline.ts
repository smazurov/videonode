import { useEffect } from 'react';
import { useStreamStore } from '../../hooks/useStreamStore';
import { useComposerStore } from '../../hooks/useComposerStore';
import { useSourceStore } from '../../hooks/useSourceStore';
import { useProcesses } from '../../hooks/useProcesses';
import { useRecordingStore } from '../../hooks/useRecordingStore';
import { useConnectionStatus } from '../../hooks/useConnectionStatus';
import type { MetricsSample } from '../../hooks/slices/streams/streamDataSlice';
import { livenessToPill } from '../../lib/pool-status';
import { formatUptime } from '../../lib/formatUptime';
import { ENCODED_FLOW, encoderActive, latestHealth, type StreamHealth } from '../../lib/stream-health';
import {
  composerStage,
  encoderStage,
  relayStage,
  summarizeStream,
  viewersStage,
  type SourceLiveness,
  type StageState,
  type StreamSummary,
} from '../../lib/stream-status';
import { resolveUpstream } from './upstream';
import { useStreamViewers, type StreamViewer } from './useStreamViewers';

const EMPTY_HISTORY: MetricsSample[] = [];

export interface PipelineNode extends StageState {
  readonly key: string;
  readonly label: string;
  readonly href?: string | undefined;
}

export interface StreamPipeline {
  readonly health: StreamHealth | undefined;
  readonly active: boolean;
  readonly history: readonly MetricsSample[];
  readonly connected: boolean;
  readonly viewers: readonly StreamViewer[];
  readonly summary: StreamSummary;
  /** Encoder pool state from the live health push, else the stored status. */
  readonly encoderState: string | undefined;
  readonly sources: readonly PipelineNode[];
  /** Composer (if any) → encoder → relay → viewers. */
  readonly chain: readonly PipelineNode[];
  readonly uptime: string | undefined;
  /** e.g. "1080p30 · audio" */
  readonly facts: string;
}

// The pipeline names every upstream hop, so both stores must be loaded even
// when the stream reads a source directly.
function useUpstreamStores(needComposers: boolean) {
  const sourcesLastUpdated = useSourceStore((s) => s.lastUpdated);
  const fetchSources = useSourceStore((s) => s.fetchSources);
  const composersLastUpdated = useComposerStore((s) => s.lastUpdated);
  const fetchComposers = useComposerStore((s) => s.fetchComposers);
  useEffect(() => {
    if (sourcesLastUpdated === null) void fetchSources();
  }, [sourcesLastUpdated, fetchSources]);
  useEffect(() => {
    if (needComposers && composersLastUpdated === null) void fetchComposers();
  }, [needComposers, composersLastUpdated, fetchComposers]);
}

/** Everything the stream page needs to show health from source to viewers. */
export function useStreamPipeline(streamId: string): StreamPipeline {
  const stream = useStreamStore((s) => s.streamsById[streamId]);
  const metrics = useStreamStore((s) => s.metricsById[streamId]);
  const metricsTs = useStreamStore((s) => s.metricsTsById[streamId]);
  const history = useStreamStore((s) => s.metricsHistoryById[streamId]) ?? EMPTY_HISTORY;
  const connected = useConnectionStatus() === 'online';
  const { processes } = useProcesses({ enabled: true });
  const viewers = useStreamViewers(streamId);
  const sourcesById = useSourceStore((s) => s.sourcesById);
  const upstream = resolveUpstream(stream ?? { stream_id: streamId });
  const composer = useComposerStore((s) => (upstream.kind === 'composer' ? s.composersById[upstream.id] : undefined));
  const recording = useRecordingStore((s) => Object.values(s.recordingsById).some((r) => r.stream_id === streamId && r.active));
  useUpstreamStores(upstream.kind === 'composer');

  const encoder = processes.find((p) => p.kind === 'encoder' && p.stream_id === streamId);
  const health = latestHealth(stream?.health, metrics?.health);
  const active = !!health && encoderActive(health);

  const sourceIds = upstream.kind === 'composer'
    ? (composer?.inputs ?? []).map((i) => i.ref.replace(/^source:/, ''))
    : [upstream.id].filter(Boolean);
  const liveness: SourceLiveness[] = sourceIds.map((id) => ({
    id,
    live: sourcesById[id]?.liveness === 'live',
    label: livenessToPill(sourcesById[id]?.liveness).label,
  }));
  const sources: PipelineNode[] = liveness.map((s) => ({
    key: `source:${s.id}`, label: s.id, href: `/sources/${s.id}`, state: s.label, tone: s.live ? 'ok' : 'warn',
  }));

  const chain: PipelineNode[] = [
    ...(upstream.kind === 'composer'
      ? [{ key: 'composer', label: upstream.id, href: `/composers/${upstream.id}`, ...composerStage(composer?.status) }]
      : []),
    { key: 'encoder', label: 'encoder', ...encoderStage(health, history[history.length - 1]?.rates[ENCODED_FLOW]) },
    { key: 'relay', label: 'relay', ...relayStage(health) },
    { key: 'viewers', label: 'viewers', ...viewersStage(viewers.length, recording) },
  ];

  const dims = upstream.kind === 'composer'
    ? { h: composer?.canvas.h, fps: composer?.canvas.fps }
    : { h: sourcesById[upstream.id]?.format?.height, fps: sourcesById[upstream.id]?.format?.fps };
  let audioFact: string | undefined;
  if (active) audioFact = (health.tracks ?? []).some((t) => t.type === 'audio') ? 'audio' : 'no audio';
  const facts = [dims.h ? `${dims.h}p${dims.fps ?? ''}` : undefined, audioFact].filter(Boolean).join(' · ');

  return {
    health,
    active,
    history,
    connected,
    viewers,
    summary: summarizeStream(connected, health, liveness, encoder?.last_error ?? undefined),
    encoderState: health?.encoder.state ?? stream?.status,
    sources,
    chain,
    uptime: encoder?.state === 'running' ? formatUptime(encoder.started_at_us, metricsTs) : undefined,
    facts,
  };
}
