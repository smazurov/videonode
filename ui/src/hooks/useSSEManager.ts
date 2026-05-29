import { useEffect } from 'react';
import type { components } from '../lib/api.generated';
import { SSEClient, type SSEStatus } from '../lib/api';
import { useDeviceStore } from './useDeviceStore';
import { dispatchEntityEvent } from './entityDispatch';
import type { EntityEvent } from './entityTypes';

type PipelineStateChangedEvent = components["schemas"]["PipelineStateChangedEvent"];

export type ConnectionStatus = 'online' | 'offline' | 'reconnecting';

interface SSEManagerOptions {
  onPipelineStateEvent?: (event: PipelineStateChangedEvent) => void;
  onConnectionStatusChange?: (status: ConnectionStatus) => void;
}

// Global SSE client instance
let globalClient: SSEClient<"/api/events"> | null = null;

// Global handlers for the two non-entity event streams.
const globalConnectionHandlers = new Set<(status: ConnectionStatus) => void>();
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

// SSEClient.on()'s keyof constraint is keyed on the generated event-name
// union; cast to a permissive view to attach the uniform 'entity' handler.
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

  globalClient.on('device-discovery', () => {
    void useDeviceStore.getState().fetchDevices();
  });

  globalClient.on('pipeline-state-changed', (data) => {
    for (const handler of globalPipelineStateHandlers) {
      handler(data);
    }
  });

  // Uniform entity envelope. One subscription handles every per-entity event
  // (lifecycle + status + metrics + consumers) by routing through the
  // dispatcher's typed ENTITY_STORES map. Adding a new entity requires only
  // updating ui/src/hooks/entityTypes.ts (literal union) +
  // ui/src/hooks/entityDispatch.ts (one map entry).
  const untyped = globalClient as unknown as UntypedSSEOn;
  untyped.on('entity', (data) => {
    dispatchEntityEvent(data as EntityEvent);
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
  const { onPipelineStateEvent, onConnectionStatusChange } = options;

  useEffect(() => {
    if (onConnectionStatusChange) {
      globalConnectionHandlers.add(onConnectionStatusChange);
    }
    if (onPipelineStateEvent) {
      globalPipelineStateHandlers.add(onPipelineStateEvent);
    }

    setupGlobalSSE();

    return () => {
      if (onConnectionStatusChange) {
        globalConnectionHandlers.delete(onConnectionStatusChange);
      }
      if (onPipelineStateEvent) {
        globalPipelineStateHandlers.delete(onPipelineStateEvent);
      }

      if (globalConnectionHandlers.size === 0 &&
          globalPipelineStateHandlers.size === 0) {
        disconnectGlobalSSE();
      }
    };
  }, [onPipelineStateEvent, onConnectionStatusChange]);

  return {
    disconnect: disconnectGlobalSSE,
    reconnect: setupGlobalSSE,
  };
}
