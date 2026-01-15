import { useEffect } from 'react';
import {
  SSEStreamLifecycleEvent,
  SSEStreamMetricsEvent,
  SSEStreamCreatedEvent,
  SSEStreamUpdatedEvent,
  SSEStreamDeletedEvent,
  StreamData,
} from '../lib/api';
import { SSEClient, SSEStatus } from '../lib/api_sse';

type ConnectionStatus = 'online' | 'offline' | 'reconnecting';

interface SSEManagerOptions {
  onStreamLifecycleEvent?: (event: SSEStreamLifecycleEvent) => void;
  onStreamMetricsEvent?: (event: SSEStreamMetricsEvent) => void;
  onConnectionStatusChange?: (status: ConnectionStatus) => void;
}

// Types for SSE event data parsing
interface StreamCreatedData {
  stream: StreamData;
  action: 'created';
  timestamp: string;
}

interface StreamDeletedData {
  stream_id: string;
  action: 'deleted';
  timestamp: string;
}

interface StreamUpdatedData {
  stream: StreamData;
  action: 'updated' | 'restarted';
  timestamp: string;
}

interface StreamMetricsData {
  stream_id: string;
  fps?: string;
  dropped_frames?: string;
  duplicate_frames?: string;
}

// Global SSE client instance
let globalClient: SSEClient | null = null;

// Global handlers for different event types
const globalConnectionHandlers = new Set<(status: ConnectionStatus) => void>();
const globalStreamLifecycleHandlers = new Set<(event: SSEStreamLifecycleEvent) => void>();
const globalStreamMetricsHandlers = new Set<(event: SSEStreamMetricsEvent) => void>();

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

  // Register typed event handlers
  globalClient.on<StreamCreatedData>('stream-created', (data) => {
    const event: SSEStreamCreatedEvent = {
      type: 'stream-created',
      stream: data.stream,
      action: data.action,
      timestamp: data.timestamp,
    };
    for (const handler of globalStreamLifecycleHandlers) {
      handler(event);
    }
  });

  globalClient.on<StreamDeletedData>('stream-deleted', (data) => {
    const event: SSEStreamDeletedEvent = {
      type: 'stream-deleted',
      stream_id: data.stream_id,
      action: data.action,
      timestamp: data.timestamp,
    };
    for (const handler of globalStreamLifecycleHandlers) {
      handler(event);
    }
  });

  globalClient.on<StreamUpdatedData>('stream-updated', (data) => {
    const event: SSEStreamUpdatedEvent = {
      type: 'stream-updated',
      stream: data.stream,
      action: data.action,
      timestamp: data.timestamp,
    };
    for (const handler of globalStreamLifecycleHandlers) {
      handler(event);
    }
  });

  globalClient.on<StreamMetricsData>('stream-metrics', (data) => {
    const event: SSEStreamMetricsEvent = {
      type: 'stream-metrics',
      stream_id: data.stream_id,
      ...(data.fps !== undefined && { fps: data.fps }),
      ...(data.dropped_frames !== undefined && { dropped_frames: data.dropped_frames }),
      ...(data.duplicate_frames !== undefined && { duplicate_frames: data.duplicate_frames }),
    };
    for (const handler of globalStreamMetricsHandlers) {
      handler(event);
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
  const { onStreamLifecycleEvent, onStreamMetricsEvent, onConnectionStatusChange } = options;

  useEffect(() => {
    // Register this component's handlers
    if (onConnectionStatusChange) {
      globalConnectionHandlers.add(onConnectionStatusChange);
    }
    if (onStreamLifecycleEvent) {
      globalStreamLifecycleHandlers.add(onStreamLifecycleEvent);
    }
    if (onStreamMetricsEvent) {
      globalStreamMetricsHandlers.add(onStreamMetricsEvent);
    }

    // Start global SSE if not already started
    setupGlobalSSE();

    return () => {
      // Unregister handlers
      if (onConnectionStatusChange) {
        globalConnectionHandlers.delete(onConnectionStatusChange);
      }
      if (onStreamLifecycleEvent) {
        globalStreamLifecycleHandlers.delete(onStreamLifecycleEvent);
      }
      if (onStreamMetricsEvent) {
        globalStreamMetricsHandlers.delete(onStreamMetricsEvent);
      }

      // Only disconnect if no handlers remain
      if (globalConnectionHandlers.size === 0 &&
          globalStreamLifecycleHandlers.size === 0 &&
          globalStreamMetricsHandlers.size === 0) {
        disconnectGlobalSSE();
      }
    };
  }, [onStreamLifecycleEvent, onStreamMetricsEvent, onConnectionStatusChange]);

  return {
    disconnect: disconnectGlobalSSE,
    reconnect: setupGlobalSSE,
  };
}
