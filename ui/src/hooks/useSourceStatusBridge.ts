import { useEffect } from 'react';
import { useStreamStore } from './useStreamStore';
import { useSourceStore, type SourceConsumerRef } from './useSourceStore';

// Stub bridge until U3 wires source-status into useSSEManager and B5 lands
// /api/sources. Today we:
//   1. Seed sources from any stream upstream ref (`source:<id>`).
//   2. Recompute downstream consumers when streams change.
// SSE-driven status updates are filed by SourceStatusBridge below once the
// backend emits `source-status` SSE frames the manager already accepts.

function parseUpstream(ref: string | undefined): { kind: 'source' | 'composer'; id: string } | null {
  if (!ref) return null;
  const [kind, ...rest] = ref.split(':');
  const id = rest.join(':');
  if (!id) return null;
  if (kind === 'source' || kind === 'composer') return { kind, id };
  return null;
}

export function useSourceStatusBridge(): void {
  const streamsById = useStreamStore((s) => s.streamsById);

  useEffect(() => {
    const upsertSource = useSourceStore.getState().upsertSource;
    const setConsumers = useSourceStore.getState().setConsumers;

    const consumerMap: Record<string, SourceConsumerRef[]> = {};
    const seen = new Set<string>();

    for (const stream of Object.values(streamsById)) {
      // The old monolithic StreamData carried device_id directly; the new
      // shape (post-B7) replaces it with `upstream`. Fall back to whatever
      // is present so the surface keeps working through the transition.
      const candidate =
        (stream as unknown as { upstream?: string }).upstream ??
        (stream as unknown as { device_id?: string }).device_id;

      const parsed = parseUpstream(candidate);
      if (!parsed) continue;

      if (parsed.kind === 'source') {
        const sourceId = parsed.id;
        seen.add(sourceId);
        upsertSource({
          id: sourceId,
          source_id: sourceId,
          test_mode: !!(stream as unknown as { test_mode?: boolean }).test_mode,
        });
        const entry: SourceConsumerRef = { kind: 'stream', id: stream.stream_id };
        consumerMap[sourceId] = consumerMap[sourceId] ? [...consumerMap[sourceId], entry] : [entry];
      }
    }

    for (const [sourceId, consumers] of Object.entries(consumerMap)) {
      setConsumers(sourceId, consumers);
    }

    // Sources with no observed consumers still need their consumer list
    // emptied so stale entries don't linger after a stream is removed.
    const sourcesById = useSourceStore.getState().sourcesById;
    for (const id of Object.keys(sourcesById)) {
      if (seen.has(id)) continue;
      if (sourcesById[id]?.consumers && sourcesById[id]!.consumers.length > 0) {
        setConsumers(id, []);
      }
    }
  }, [streamsById]);
}
