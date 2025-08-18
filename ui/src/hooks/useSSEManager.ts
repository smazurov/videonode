import { useEffect } from 'react';
import { SSEStreamEvent, SSESystemEvent, SSEStreamLifecycleEvent, SSEStreamMetricsEvent } from '../lib/api';

type ConnectionStatus = 'online' | 'offline' | 'reconnecting';

interface SSEManagerOptions {
  onStreamEvent?: (event: SSEStreamEvent) => void;
  onStreamLifecycleEvent?: (event: SSEStreamLifecycleEvent) => void;
  onStreamMetricsEvent?: (event: SSEStreamMetricsEvent) => void;
  onSystemEvent?: (event: SSESystemEvent) => void;
  onConnectionStatusChange?: (status: ConnectionStatus) => void;
}

// Constants to avoid duplication
const INITIAL_RECONNECT_DELAY = 5000;
const MAX_RECONNECT_DELAY = 60000;

// Global SSE manager state
let globalEventSource: EventSource | null = null;
let globalReconnectTimeout: number | null = null;
let globalReconnectDelay = INITIAL_RECONNECT_DELAY;

// Global handlers for different event types
const globalConnectionHandlers = new Set<(status: ConnectionStatus) => void>();
const globalStreamEventHandlers = new Set<(event: SSEStreamEvent) => void>();
const globalStreamLifecycleHandlers = new Set<(event: SSEStreamLifecycleEvent) => void>();
const globalStreamMetricsHandlers = new Set<(event: SSEStreamMetricsEvent) => void>();
const globalSystemEventHandlers = new Set<(event: SSESystemEvent) => void>();

function setupGlobalSSE(): void {
  if (globalEventSource) return; // Already connected

  const credentials = localStorage.getItem('auth_credentials');
  if (!credentials) return;

  try {
    const eventSource = new EventSource(`http://localhost:8090/api/events?auth=${encodeURIComponent(credentials)}`, {
      withCredentials: false,
    });

    // Handle stream lifecycle events (create/delete)
    eventSource.addEventListener('stream-created', (event: MessageEvent) => {
      try {
        const data = JSON.parse(event.data as string);
        const streamEvent: SSEStreamLifecycleEvent = { 
          type: 'stream-created',
          ...data 
        };
        for (const handler of globalStreamLifecycleHandlers) {
          handler(streamEvent);
        }
      } catch (error) {
        console.error('Error parsing stream-created event:', error);
      }
    });

    eventSource.addEventListener('stream-deleted', (event: MessageEvent) => {
      try {
        const data = JSON.parse(event.data as string);
        const streamEvent: SSEStreamLifecycleEvent = { 
          type: 'stream-deleted',
          ...data 
        };
        for (const handler of globalStreamLifecycleHandlers) {
          handler(streamEvent);
        }
      } catch (error) {
        console.error('Error parsing stream-deleted event:', error);
      }
    });

    // Handle stream metrics events
    eventSource.addEventListener('stream-metrics', (event: MessageEvent) => {
      try {
        const data = JSON.parse(event.data as string);
        const metricsEvent: SSEStreamMetricsEvent = { 
          type: 'stream_metrics',
          ...data 
        };
        for (const handler of globalStreamMetricsHandlers) {
          handler(metricsEvent);
        }
      } catch (error) {
        console.error('Error parsing stream-metrics event:', error);
      }
    });

    // Handle system events globally
    eventSource.addEventListener('system-status', (event: MessageEvent) => {
      console.log('SSE system-status event received:', event.data);
      try {
        const data = JSON.parse(event.data as string) as Omit<SSESystemEvent, 'type'>;
        const systemEvent: SSESystemEvent = { type: 'system-status', ...data };
        for (const handler of globalSystemEventHandlers) {
          handler(systemEvent);
        }
      } catch (error) {
        console.error('Error parsing system-status event:', error);
      }
    });

    eventSource.onerror = () => {
      console.error('SSE connection error');
      for (const handler of globalConnectionHandlers) {
        handler('reconnecting');
      }
      eventSource.close();
      globalEventSource = null;
      
      // Clear any existing reconnect timeout
      if (globalReconnectTimeout) {
        window.clearTimeout(globalReconnectTimeout);
      }
      
      // Exponential backoff with max delay
      const currentDelay = globalReconnectDelay;
      console.log(`Reconnecting in ${currentDelay / 1000} seconds...`);
      
      globalReconnectTimeout = window.setTimeout(() => {
        setupGlobalSSE();
        // Double the delay for next attempt, max 60 seconds
        globalReconnectDelay = Math.min(currentDelay * 2, MAX_RECONNECT_DELAY);
      }, currentDelay);
    };

    globalEventSource = eventSource;
  } catch (error) {
    console.error('Failed to setup SSE:', error);
      for (const handler of globalConnectionHandlers) {
        handler('online');
      }
  }
}

function disconnectGlobalSSE(): void {
  if (globalReconnectTimeout) {
    window.clearTimeout(globalReconnectTimeout);
    globalReconnectTimeout = null;
  }
  if (globalEventSource) {
    globalEventSource.close();
    globalEventSource = null;
  }
}

export function useSSEManager(options: SSEManagerOptions = {}) {
  const { onStreamEvent, onStreamLifecycleEvent, onStreamMetricsEvent, onSystemEvent, onConnectionStatusChange } = options;

  useEffect(() => {
    // Register this component's handlers
    if (onConnectionStatusChange) {
      globalConnectionHandlers.add(onConnectionStatusChange);
    }
    if (onStreamEvent) {
      globalStreamEventHandlers.add(onStreamEvent);
    }
    if (onStreamLifecycleEvent) {
      globalStreamLifecycleHandlers.add(onStreamLifecycleEvent);
    }
    if (onStreamMetricsEvent) {
      globalStreamMetricsHandlers.add(onStreamMetricsEvent);
    }
    if (onSystemEvent) {
      globalSystemEventHandlers.add(onSystemEvent);
    }

    // Start global SSE if not already started
    setupGlobalSSE();

    return () => {
      // Unregister handlers
      if (onConnectionStatusChange) {
        globalConnectionHandlers.delete(onConnectionStatusChange);
      }
      if (onStreamEvent) {
        globalStreamEventHandlers.delete(onStreamEvent);
      }
      if (onStreamLifecycleEvent) {
        globalStreamLifecycleHandlers.delete(onStreamLifecycleEvent);
      }
      if (onStreamMetricsEvent) {
        globalStreamMetricsHandlers.delete(onStreamMetricsEvent);
      }
      if (onSystemEvent) {
        globalSystemEventHandlers.delete(onSystemEvent);
      }

      // Only disconnect if no handlers remain
      if (globalConnectionHandlers.size === 0 && 
          globalStreamEventHandlers.size === 0 && 
          globalStreamLifecycleHandlers.size === 0 &&
          globalStreamMetricsHandlers.size === 0 &&
          globalSystemEventHandlers.size === 0) {
        disconnectGlobalSSE();
      }
    };
  }, [onStreamEvent, onStreamLifecycleEvent, onStreamMetricsEvent, onSystemEvent, onConnectionStatusChange]);

  return {
    disconnect: disconnectGlobalSSE,
    reconnect: setupGlobalSSE,
  };
}