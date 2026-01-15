import { useEffect, useRef, useState } from 'react';
import { SSEClient, SSEStatus } from '../lib/api_sse';

export interface UseSSEOptions {
  endpoint: string;
  onMessage?: (event: MessageEvent) => void;
  onConnect?: () => void;
  enabled?: boolean;
}

export interface UseSSEResult {
  status: SSEStatus;
  reconnect: () => void;
  disconnect: () => void;
}

export function useSSE(options: UseSSEOptions): UseSSEResult {
  const { endpoint, onMessage, onConnect, enabled = true } = options;
  const [status, setStatus] = useState<SSEStatus>('disconnected');
  const clientRef = useRef<SSEClient | null>(null);

  // Store callbacks in refs to avoid recreating client on callback changes
  const onMessageRef = useRef(onMessage);
  const onConnectRef = useRef(onConnect);

  // Sync refs in effect to avoid updating during render
  useEffect(() => {
    onMessageRef.current = onMessage;
    onConnectRef.current = onConnect;
  });

  useEffect(() => {
    if (!enabled) return;

    const client = new SSEClient({
      endpoint,
      onStatusChange: setStatus,
      onConnect: () => onConnectRef.current?.(),
    });

    if (onMessageRef.current) {
      client.onMessage((event) => onMessageRef.current?.(event));
    }

    clientRef.current = client;
    client.connect();

    return () => {
      client.disconnect();
      clientRef.current = null;
    };
  }, [endpoint, enabled]);

  return {
    status,
    reconnect: () => clientRef.current?.connect(),
    disconnect: () => clientRef.current?.disconnect(),
  };
}
