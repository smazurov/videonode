import { useEffect } from 'react';
import {
  API_BASE_URL,
  SSEStreamLifecycleEvent,
  SSEStreamMetricsEvent,
  SSEStreamCreatedEvent,
  SSEStreamUpdatedEvent,
  SSEStreamDeletedEvent
} from '../lib/api';

type ConnectionStatus = 'online' | 'offline' | 'reconnecting';

interface SSEManagerOptions {
  onStreamLifecycleEvent?: (event: SSEStreamLifecycleEvent) => void;
  onStreamMetricsEvent?: (event: SSEStreamMetricsEvent) => void;
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
const globalStreamLifecycleHandlers = new Set<(event: SSEStreamLifecycleEvent) => void>();
const globalStreamMetricsHandlers = new Set<(event: SSEStreamMetricsEvent) => void>();

function setupGlobalSSE(): void {
  if (globalEventSource) return; // Already connected

  const credentials = localStorage.getItem('auth_credentials');
  if (!credentials) return;

  try {
    const sseUrl = `${API_BASE_URL}/api/events?auth=${encodeURIComponent(credentials)}`;
    const eventSource = new EventSource(sseUrl, {
      withCredentials: false,
    });

    // Connection opened successfully
    eventSource.onopen = () => {
      console.log('SSE connection established');
      globalReconnectDelay = INITIAL_RECONNECT_DELAY; // Reset reconnect delay on successful connection
      for (const handler of globalConnectionHandlers) {
        handler('online');
      }
    };

    // Handle stream lifecycle events (create/delete)
    eventSource.addEventListener('stream-created', (event: MessageEvent) => {
      try {
        const data = JSON.parse(event.data as string) as SSEStreamCreatedEvent;
        const streamEvent: SSEStreamLifecycleEvent = data;
        for (const handler of globalStreamLifecycleHandlers) {
          handler(streamEvent);
        }
      } catch (error) {
        console.error('Error parsing stream-created event:', error);
      }
    });

    eventSource.addEventListener('stream-deleted', (event: MessageEvent) => {
      try {
        const data = JSON.parse(event.data as string) as SSEStreamDeletedEvent;
        const streamEvent: SSEStreamLifecycleEvent = data;
        for (const handler of globalStreamLifecycleHandlers) {
          handler(streamEvent);
        }
      } catch (error) {
        console.error('Error parsing stream-deleted event:', error);
      }
    });

    eventSource.addEventListener('stream-updated', (event: MessageEvent) => {
      try {
        const data = JSON.parse(event.data as string) as SSEStreamUpdatedEvent;
        const streamEvent: SSEStreamLifecycleEvent = data;
        for (const handler of globalStreamLifecycleHandlers) {
          handler(streamEvent);
        }
      } catch (error) {
        console.error('Error parsing stream-updated event:', error);
      }
    });

    // Handle stream metrics events
    eventSource.addEventListener('stream-metrics', (event: MessageEvent) => {
      try {
        const data = JSON.parse(event.data as string) as Omit<SSEStreamMetricsEvent, 'type'>;
        const metricsEvent: SSEStreamMetricsEvent = { 
          type: 'stream-metrics',
          ...data 
        };
        for (const handler of globalStreamMetricsHandlers) {
          handler(metricsEvent);
        }
      } catch (error) {
        console.error('Error parsing stream-metrics event:', error);
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
        handler('offline');
      }
    }}

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