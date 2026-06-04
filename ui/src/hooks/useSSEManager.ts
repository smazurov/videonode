import { useEffect } from 'react';
import type { components } from '../lib/api.generated';
import { SSEClient, type SSEStatus } from '../lib/api';
import { useDeviceStore } from './useDeviceStore';
import { dispatchEntityEvent } from './entityDispatch';
import type { EntityEvent } from './entityTypes';
import { checkServerVersion } from '../lib/versionWatch';

type PipelineStateChangedEvent = components["schemas"]["PipelineStateChangedEvent"];
type ProcessesEvent = components["schemas"]["ProcessesEvent"];
type ProcessRemovedEvent = components["schemas"]["ProcessRemovedEvent"];

export type ConnectionStatus = 'online' | 'offline' | 'reconnecting';

interface SSEManagerOptions {
  onPipelineStateEvent?: (event: PipelineStateChangedEvent) => void;
  onConnectionStatusChange?: (status: ConnectionStatus) => void;
}

// Global SSE client instance
let globalClient: SSEClient<"/api/events"> | null = null;
// Backstop poll for the device list (see setupGlobalSSE).
let deviceFallbackTimer: number | null = null;
// Re-check the build version when the tab regains focus — the refresh-on-focus
// path, and the moment a busy-deferred update can finish.
function onVisibilityCheck(): void {
  if (document.visibilityState === 'visible') void checkServerVersion();
}
// Count of app-shell pins holding the connection open for the whole session
// (see useAppSSEConnection). While > 0 the client stays connected even when
// every per-route subscriber has unmounted, so navigating between pages no
// longer tears down and reconnects the stream.
let appConnectionPins = 0;

// Global handlers for the non-entity event streams.
const globalConnectionHandlers = new Set<(status: ConnectionStatus) => void>();
const globalPipelineStateHandlers = new Set<(event: PipelineStateChangedEvent) => void>();
const globalProcessesHandlers = new Set<(event: ProcessesEvent) => void>();
const globalProcessRemovedHandlers = new Set<(event: ProcessRemovedEvent) => void>();

function teardownIfIdle(): void {
  if (
    appConnectionPins === 0 &&
    globalConnectionHandlers.size === 0 &&
    globalPipelineStateHandlers.size === 0 &&
    globalProcessesHandlers.size === 0 &&
    globalProcessRemovedHandlers.size === 0
  ) {
    disconnectGlobalSSE();
  }
}

// subscribeProcesses attaches a handler for the dedicated process event
// stream (per-process state transitions + 2s stats samples) and ensures the
// shared SSE client is alive. Returns an unsubscribe. The steady-state source
// of truth for the Processes panel; useProcesses keeps only an initial/
// reconnect fetch behind it.
export function subscribeProcesses(fn: (event: ProcessesEvent) => void): () => void {
  globalProcessesHandlers.add(fn);
  setupGlobalSSE();
  return () => {
    globalProcessesHandlers.delete(fn);
    teardownIfIdle();
  };
}

// subscribeProcessRemoved attaches a handler for the process-removed signal,
// fired when a supervised process leaves the pool (it emits no further state
// or stats events). Returns an unsubscribe.
export function subscribeProcessRemoved(fn: (event: ProcessRemovedEvent) => void): () => void {
  globalProcessRemovedHandlers.add(fn);
  setupGlobalSSE();
  return () => {
    globalProcessRemovedHandlers.delete(fn);
    teardownIfIdle();
  };
}

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

// subscribeConnectionStatus is a non-hook handle onto the same connection
// authority useConnectionStatus reads, for module-level consumers (the shared
// pollers) that need to pause when the rig goes offline. Ensures the global
// SSE client is alive and returns an unsubscribe.
export function subscribeConnectionStatus(
  fn: (status: ConnectionStatus) => void,
): () => void {
  globalConnectionHandlers.add(fn);
  setupGlobalSSE();
  return () => {
    globalConnectionHandlers.delete(fn);
  };
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
    heartbeatWatchdog: true,
    onStatusChange: (status) => {
      const mapped = mapStatus(status);
      for (const handler of globalConnectionHandlers) {
        handler(mapped);
      }
    },
    onConnect: () => {
      // The backend only broadcasts the device snapshot once at daemon
      // startup, so a client connecting later never receives it over the
      // stream. Pull the current list on every (re)connect to seed it and to
      // resync any hotplug deltas missed during a disconnect.
      void useDeviceStore.getState().fetchDevices();
      // A reconnect usually means the daemon restarted — the moment to check
      // whether it's a new build and reload (or flag a pending update).
      void checkServerVersion();
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

  globalClient.on('processes', (data) => {
    for (const handler of globalProcessesHandlers) {
      handler(data);
    }
  });

  globalClient.on('process-removed', (data) => {
    for (const handler of globalProcessRemovedHandlers) {
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

  // Fallback poll for the device list: the device-discovery event is the
  // primary freshness signal, but a missed event would leave the list stale.
  // Re-fetch every 30s as a backstop.
  deviceFallbackTimer = window.setInterval(() => {
    void useDeviceStore.getState().fetchDevices();
  }, 30_000);

  document.addEventListener('visibilitychange', onVisibilityCheck);

  globalClient.connect();
}

function disconnectGlobalSSE(): void {
  if (deviceFallbackTimer != null) {
    window.clearInterval(deviceFallbackTimer);
    deviceFallbackTimer = null;
  }
  document.removeEventListener('visibilitychange', onVisibilityCheck);
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
      onConnectionStatusChange(getGlobalConnectionStatus());
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

      teardownIfIdle();
    };
  }, [onPipelineStateEvent, onConnectionStatusChange]);

  return {
    disconnect: disconnectGlobalSSE,
    reconnect: setupGlobalSSE,
  };
}

// useAppSSEConnection pins the shared SSE client open for the entire
// authenticated app session. Mount it once at the persistent route shell
// (Root) so per-route subscribers that come and go on navigation (InfoBar,
// page-level managers) no longer drive the connection up and down — they just
// attach and detach their handlers against an already-open stream.
export function useAppSSEConnection(): void {
  useEffect(() => {
    appConnectionPins += 1;
    setupGlobalSSE();
    return () => {
      appConnectionPins -= 1;
      teardownIfIdle();
    };
  }, []);
}
