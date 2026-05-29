import { useEffect } from 'react';
import type { components } from '../lib/api.generated';
import { SSEClient, type SSEStatus } from '../lib/api';
import { useDeviceStore } from './useDeviceStore';
import { useSourceStore } from './useSourceStore';
import { useComposerStore } from './useComposerStore';
import type { Source as SourceData, Composer as ComposerData, LayoutSlot as ComposerLayoutSlot } from './slices/types';
import { dispatchEntityEvent } from './entityDispatch';
import type { EntityEvent } from './entityTypes';

type StreamCreatedEvent = components["schemas"]["StreamCreatedEvent"];
type StreamUpdatedEvent = components["schemas"]["StreamUpdatedEvent"];
type StreamDeletedEvent = components["schemas"]["StreamDeletedEvent"];
type StreamMetricsEvent = components["schemas"]["StreamMetricsEvent"];
type StreamStateChangedEvent = components["schemas"]["StreamStateChangedEvent"];
type PipelineStateChangedEvent = components["schemas"]["PipelineStateChangedEvent"];

export type TaggedStreamCreatedEvent = StreamCreatedEvent & { type: 'stream-created' };
export type TaggedStreamUpdatedEvent = StreamUpdatedEvent & { type: 'stream-updated' };
export type TaggedStreamDeletedEvent = StreamDeletedEvent & { type: 'stream-deleted' };
export type TaggedStreamStateChangedEvent = StreamStateChangedEvent & { type: 'stream-state-changed' };
export type StreamLifecycleEvent =
  | TaggedStreamCreatedEvent
  | TaggedStreamUpdatedEvent
  | TaggedStreamDeletedEvent;

// Source/composer event payloads are stubbed locally until U1 regenerates
// the OpenAPI types. Shapes match the canonical plan; once regen lands, swap
// these for components["schemas"]["..."] without touching the handler logic.
export interface SourceCreatedEvent { action: string; source: SourceData; timestamp: string }
export interface SourceUpdatedEvent { action: string; source: SourceData; timestamp: string }
export interface SourceDeletedEvent { action: string; source_id: string; timestamp: string }

export interface ComposerCreatedEvent { action: string; composer: ComposerData; timestamp: string }
export interface ComposerUpdatedEvent { action: string; composer: ComposerData; timestamp: string }
export interface ComposerDeletedEvent { action: string; composer_id: string; timestamp: string }
export interface ComposerLayoutChangedEvent {
  action: string;
  composer_id: string;
  layout: ComposerLayoutSlot[];
  timestamp: string;
}

export type TaggedSourceCreatedEvent = SourceCreatedEvent & { type: 'source-created' };
export type TaggedSourceUpdatedEvent = SourceUpdatedEvent & { type: 'source-updated' };
export type TaggedSourceDeletedEvent = SourceDeletedEvent & { type: 'source-deleted' };
export type SourceLifecycleEvent =
  | TaggedSourceCreatedEvent
  | TaggedSourceUpdatedEvent
  | TaggedSourceDeletedEvent;

export type TaggedComposerCreatedEvent = ComposerCreatedEvent & { type: 'composer-created' };
export type TaggedComposerUpdatedEvent = ComposerUpdatedEvent & { type: 'composer-updated' };
export type TaggedComposerDeletedEvent = ComposerDeletedEvent & { type: 'composer-deleted' };
export type TaggedComposerLayoutChangedEvent = ComposerLayoutChangedEvent & { type: 'composer-layout-changed' };
export type ComposerLifecycleEvent =
  | TaggedComposerCreatedEvent
  | TaggedComposerUpdatedEvent
  | TaggedComposerDeletedEvent
  | TaggedComposerLayoutChangedEvent;

export type ConnectionStatus = 'online' | 'offline' | 'reconnecting';

interface SSEManagerOptions {
  onStreamLifecycleEvent?: (event: StreamLifecycleEvent) => void;
  onStreamMetricsEvent?: (event: StreamMetricsEvent) => void;
  onStreamStateEvent?: (event: TaggedStreamStateChangedEvent) => void;
  onSourceLifecycleEvent?: (event: SourceLifecycleEvent) => void;
  onComposerLifecycleEvent?: (event: ComposerLifecycleEvent) => void;
  onPipelineStateEvent?: (event: PipelineStateChangedEvent) => void;
  onConnectionStatusChange?: (status: ConnectionStatus) => void;
}

// Global SSE client instance
let globalClient: SSEClient<"/api/events"> | null = null;

// Global handlers for different event types
const globalConnectionHandlers = new Set<(status: ConnectionStatus) => void>();
const globalStreamLifecycleHandlers = new Set<(event: StreamLifecycleEvent) => void>();
const globalStreamMetricsHandlers = new Set<(event: StreamMetricsEvent) => void>();
export const globalStreamStateHandlers = new Set<(event: TaggedStreamStateChangedEvent) => void>();
const globalSourceLifecycleHandlers = new Set<(event: SourceLifecycleEvent) => void>();
const globalComposerLifecycleHandlers = new Set<(event: ComposerLifecycleEvent) => void>();
const globalPipelineStateHandlers = new Set<(event: PipelineStateChangedEvent) => void>();

function mapStatus(status: SSEStatus): ConnectionStatus {
  switch (status) {
    case 'connected': return 'online';
    // An EventSource attempt is actively in flight — surface as 'reconnecting'
    // so the UI only says "reconnecting" while genuinely trying. The idle
    // backoff wait stays 'disconnected' → 'offline' ("Disconnected").
    case 'connecting':
    case 'reconnecting': return 'reconnecting';
    default: return 'offline';
  }
}

// Current connection status of the shared SSE client, for seeding local state
// without waiting for the next onStatusChange event. 'online' is the
// optimistic default before the client has connected, so consumers that mount
// during a healthy session don't flash a disconnected indicator.
export function getGlobalConnectionStatus(): ConnectionStatus {
  return globalClient ? mapStatus(globalClient.getStatus()) : 'online';
}

// Until U1 regenerates api.generated.ts with source/composer events, the
// SSEClient.on() overload's keyof constraint rejects the new event names.
// Cast to a permissive view that accepts arbitrary event names + payloads.
type UntypedSSEOn = {
  on: (eventType: string, handler: (data: unknown) => void) => void;
};

function setupGlobalSSE(): void {
  if (globalClient) return;

  globalClient = new SSEClient({
    endpoint: '/api/events',
    onStatusChange: (status) => {
      const mapped = mapStatus(status);
      for (const handler of globalConnectionHandlers) {
        handler(mapped);
      }
    },
    onConnect: () => {
      console.log('SSE connection established');
    },
  });

  globalClient.on('stream-created', (data) => {
    const event = { ...data, type: 'stream-created' as const };
    for (const handler of globalStreamLifecycleHandlers) {
      handler(event);
    }
  });

  globalClient.on('stream-deleted', (data) => {
    const event = { ...data, type: 'stream-deleted' as const };
    for (const handler of globalStreamLifecycleHandlers) {
      handler(event);
    }
  });

  globalClient.on('stream-updated', (data) => {
    const event = { ...data, type: 'stream-updated' as const };
    for (const handler of globalStreamLifecycleHandlers) {
      handler(event);
    }
  });

  globalClient.on('stream-metrics', (data) => {
    for (const handler of globalStreamMetricsHandlers) {
      handler(data);
    }
  });

  globalClient.on('stream-state-changed', (data) => {
    const event = { ...data, type: 'stream-state-changed' as const };
    for (const handler of globalStreamStateHandlers) {
      handler(event);
    }
  });

  globalClient.on('device-discovery', () => {
    void useDeviceStore.getState().fetchDevices();
  });

  globalClient.on('pipeline-state-changed', (data) => {
    for (const handler of globalPipelineStateHandlers) {
      handler(data);
    }
  });

  const untyped = globalClient as unknown as UntypedSSEOn;

  // Uniform entity envelope. One subscription handles every per-entity
  // event (lifecycle + status + metrics + consumers) by routing through
  // the dispatcher's typed ENTITY_STORES map. Adding a new entity
  // requires only updating ui/src/hooks/entityTypes.ts (literal union)
  // + ui/src/hooks/entityDispatch.ts (one map entry). The legacy
  // per-type handlers below continue to fire during the dual-publish
  // migration; they'll be removed in Step 5.
  untyped.on('entity', (data) => {
    dispatchEntityEvent(data as EntityEvent);
  });

  untyped.on('source-created', (data) => {
    const payload = data as SourceCreatedEvent;
    useSourceStore.getState().addSource(payload.source);
    const event: TaggedSourceCreatedEvent = { ...payload, type: 'source-created' };
    for (const handler of globalSourceLifecycleHandlers) handler(event);
  });

  untyped.on('source-updated', (data) => {
    const payload = data as SourceUpdatedEvent;
    useSourceStore.getState().addSource(payload.source);
    const event: TaggedSourceUpdatedEvent = { ...payload, type: 'source-updated' };
    for (const handler of globalSourceLifecycleHandlers) handler(event);
  });

  untyped.on('source-deleted', (data) => {
    const payload = data as SourceDeletedEvent;
    useSourceStore.getState().removeSource(payload.source_id);
    const event: TaggedSourceDeletedEvent = { ...payload, type: 'source-deleted' };
    for (const handler of globalSourceLifecycleHandlers) handler(event);
  });

  untyped.on('composer-created', (data) => {
    const payload = data as ComposerCreatedEvent;
    useComposerStore.getState().addComposer(payload.composer);
    const event: TaggedComposerCreatedEvent = { ...payload, type: 'composer-created' };
    for (const handler of globalComposerLifecycleHandlers) handler(event);
  });

  untyped.on('composer-updated', (data) => {
    const payload = data as ComposerUpdatedEvent;
    useComposerStore.getState().addComposer(payload.composer);
    const event: TaggedComposerUpdatedEvent = { ...payload, type: 'composer-updated' };
    for (const handler of globalComposerLifecycleHandlers) handler(event);
  });

  untyped.on('composer-deleted', (data) => {
    const payload = data as ComposerDeletedEvent;
    useComposerStore.getState().removeComposer(payload.composer_id);
    const event: TaggedComposerDeletedEvent = { ...payload, type: 'composer-deleted' };
    for (const handler of globalComposerLifecycleHandlers) handler(event);
  });

  untyped.on('composer-layout-changed', (data) => {
    const payload = data as ComposerLayoutChangedEvent;
    const composer = useComposerStore.getState().getComposerById(payload.composer_id);
    if (composer) {
      useComposerStore.getState().addComposer({ ...composer, layout: payload.layout });
    }
    const event: TaggedComposerLayoutChangedEvent = { ...payload, type: 'composer-layout-changed' };
    for (const handler of globalComposerLifecycleHandlers) handler(event);
  });

  globalClient.connect();
}

function disconnectGlobalSSE(): void {
  if (globalClient) {
    globalClient.disconnect();
    globalClient = null;
  }
}

export function useSSEManager(options: SSEManagerOptions = {}) {
  const {
    onStreamLifecycleEvent,
    onStreamMetricsEvent,
    onStreamStateEvent,
    onSourceLifecycleEvent,
    onComposerLifecycleEvent,
    onPipelineStateEvent,
    onConnectionStatusChange,
  } = options;

  useEffect(() => {
    if (onConnectionStatusChange) {
      globalConnectionHandlers.add(onConnectionStatusChange);
    }
    if (onStreamLifecycleEvent) {
      globalStreamLifecycleHandlers.add(onStreamLifecycleEvent);
    }
    if (onStreamMetricsEvent) {
      globalStreamMetricsHandlers.add(onStreamMetricsEvent);
    }
    if (onStreamStateEvent) {
      globalStreamStateHandlers.add(onStreamStateEvent);
    }
    if (onSourceLifecycleEvent) {
      globalSourceLifecycleHandlers.add(onSourceLifecycleEvent);
    }
    if (onComposerLifecycleEvent) {
      globalComposerLifecycleHandlers.add(onComposerLifecycleEvent);
    }
    if (onPipelineStateEvent) {
      globalPipelineStateHandlers.add(onPipelineStateEvent);
    }

    setupGlobalSSE();

    return () => {
      if (onConnectionStatusChange) {
        globalConnectionHandlers.delete(onConnectionStatusChange);
      }
      if (onStreamLifecycleEvent) {
        globalStreamLifecycleHandlers.delete(onStreamLifecycleEvent);
      }
      if (onStreamMetricsEvent) {
        globalStreamMetricsHandlers.delete(onStreamMetricsEvent);
      }
      if (onStreamStateEvent) {
        globalStreamStateHandlers.delete(onStreamStateEvent);
      }
      if (onSourceLifecycleEvent) {
        globalSourceLifecycleHandlers.delete(onSourceLifecycleEvent);
      }
      if (onComposerLifecycleEvent) {
        globalComposerLifecycleHandlers.delete(onComposerLifecycleEvent);
      }
      if (onPipelineStateEvent) {
        globalPipelineStateHandlers.delete(onPipelineStateEvent);
      }

      if (globalConnectionHandlers.size === 0 &&
          globalStreamLifecycleHandlers.size === 0 &&
          globalStreamMetricsHandlers.size === 0 &&
          globalStreamStateHandlers.size === 0 &&
          globalSourceLifecycleHandlers.size === 0 &&
          globalComposerLifecycleHandlers.size === 0 &&
          globalPipelineStateHandlers.size === 0) {
        disconnectGlobalSSE();
      }
    };
  }, [
    onStreamLifecycleEvent,
    onStreamMetricsEvent,
    onStreamStateEvent,
    onSourceLifecycleEvent,
    onComposerLifecycleEvent,
    onPipelineStateEvent,
    onConnectionStatusChange,
  ]);

  return {
    disconnect: disconnectGlobalSSE,
    reconnect: setupGlobalSSE,
  };
}
