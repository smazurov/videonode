import { useEffect } from 'react';
import type { components } from '../lib/api.generated';
import { SSEClient, type SSEStatus } from '../lib/api';
import { useDeviceStore } from './useDeviceStore';

type StreamCreatedEvent = components["schemas"]["StreamCreatedEvent"];
type StreamUpdatedEvent = components["schemas"]["StreamUpdatedEvent"];
type StreamDeletedEvent = components["schemas"]["StreamDeletedEvent"];
type StreamMetricsEvent = components["schemas"]["StreamMetricsEvent"];
type StreamStateChangedEvent = components["schemas"]["StreamStateChangedEvent"];
type CanvasRestartedEvent = components["schemas"]["CanvasRestartedEvent"];
type PipelineStateChangedEvent = components["schemas"]["PipelineStateChangedEvent"];

export type TaggedStreamCreatedEvent = StreamCreatedEvent & { type: 'stream-created' };
export type TaggedStreamUpdatedEvent = StreamUpdatedEvent & { type: 'stream-updated' };
export type TaggedStreamDeletedEvent = StreamDeletedEvent & { type: 'stream-deleted' };
export type TaggedStreamStateChangedEvent = StreamStateChangedEvent & { type: 'stream-state-changed' };
export type TaggedCanvasRestartedEvent = CanvasRestartedEvent & { type: 'canvas-restarted' };
export type StreamLifecycleEvent =
  | TaggedStreamCreatedEvent
  | TaggedStreamUpdatedEvent
  | TaggedStreamDeletedEvent
  | TaggedCanvasRestartedEvent;

type ConnectionStatus = 'online' | 'offline' | 'reconnecting';

interface SSEManagerOptions {
  onStreamLifecycleEvent?: (event: StreamLifecycleEvent) => void;
  onStreamMetricsEvent?: (event: StreamMetricsEvent) => void;
  onStreamStateEvent?: (event: TaggedStreamStateChangedEvent) => void;
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
const globalPipelineStateHandlers = new Set<(event: PipelineStateChangedEvent) => void>();

function mapStatus(status: SSEStatus): ConnectionStatus {
  switch (status) {
    case 'connected': return 'online';
    case 'reconnecting': return 'reconnecting';
    default: return 'offline';
  }
}

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

  globalClient.on('canvas-restarted', (data) => {
    const event = { ...data, type: 'canvas-restarted' as const };
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

  globalClient.connect();
}

function disconnectGlobalSSE(): void {
  if (globalClient) {
    globalClient.disconnect();
    globalClient = null;
  }
}

export function useSSEManager(options: SSEManagerOptions = {}) {
  const { onStreamLifecycleEvent, onStreamMetricsEvent, onStreamStateEvent, onPipelineStateEvent, onConnectionStatusChange } = options;

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
      if (onPipelineStateEvent) {
        globalPipelineStateHandlers.delete(onPipelineStateEvent);
      }

      if (globalConnectionHandlers.size === 0 &&
          globalStreamLifecycleHandlers.size === 0 &&
          globalStreamMetricsHandlers.size === 0 &&
          globalStreamStateHandlers.size === 0 &&
          globalPipelineStateHandlers.size === 0) {
        disconnectGlobalSSE();
      }
    };
  }, [onStreamLifecycleEvent, onStreamMetricsEvent, onStreamStateEvent, onPipelineStateEvent, onConnectionStatusChange]);

  return {
    disconnect: disconnectGlobalSSE,
    reconnect: setupGlobalSSE,
  };
}
