import { toast } from 'react-hot-toast';
import { API_BASE_URL } from './api';
import { clearAuthState } from '../hooks/useAuthStore';

export type SSEStatus = 'connecting' | 'connected' | 'disconnected' | 'reconnecting';

const INITIAL_RECONNECT_DELAY = 5000;
const MAX_RECONNECT_DELAY = 60000;

export interface SSEClientConfig {
  endpoint: string;
  onStatusChange?: (status: SSEStatus) => void;
  onConnect?: () => void;
  onError?: (willReconnect: boolean) => void;
}

type MessageHandler = (event: MessageEvent) => void;
type TypedEventHandler<T> = (data: T) => void;

export class SSEClient {
  private eventSource: EventSource | null = null;
  private reconnectTimeout: number | null = null;
  private reconnectDelay = INITIAL_RECONNECT_DELAY;
  private messageHandler: MessageHandler | null = null;
  private readonly typedHandlers: Map<string, TypedEventHandler<unknown>> = new Map();
  private status: SSEStatus = 'disconnected';

  constructor(private readonly config: SSEClientConfig) {}

  connect(): void {
    if (this.eventSource) return;

    const credentials = localStorage.getItem('auth_credentials');
    if (!credentials) {
      this.setStatus('disconnected');
      return;
    }

    this.setStatus('connecting');

    const sseUrl = `${API_BASE_URL}${this.config.endpoint}?auth=${encodeURIComponent(credentials)}`;
    this.eventSource = new EventSource(sseUrl);

    this.eventSource.onopen = () => {
      this.reconnectDelay = INITIAL_RECONNECT_DELAY;
      this.setStatus('connected');
      this.config.onConnect?.();
    };

    if (this.messageHandler) {
      this.eventSource.onmessage = this.messageHandler;
    }

    // Re-attach typed event handlers
    for (const [eventType, handler] of this.typedHandlers) {
      this.attachTypedHandler(eventType, handler);
    }

    this.eventSource.onerror = async () => {
      this.setStatus('reconnecting');
      this.eventSource?.close();
      this.eventSource = null;

      const authFailed = await this.verifyAuthOrRedirect();
      if (authFailed) {
        this.config.onError?.(false);
        return;
      }

      this.config.onError?.(true);
      this.scheduleReconnect();
    };
  }

  disconnect(): void {
    if (this.reconnectTimeout) {
      window.clearTimeout(this.reconnectTimeout);
      this.reconnectTimeout = null;
    }
    if (this.eventSource) {
      this.eventSource.close();
      this.eventSource = null;
    }
    this.setStatus('disconnected');
  }

  getEventSource(): EventSource | null {
    return this.eventSource;
  }

  getStatus(): SSEStatus {
    return this.status;
  }

  onMessage(handler: MessageHandler): void {
    this.messageHandler = handler;
    if (this.eventSource) {
      this.eventSource.onmessage = handler;
    }
  }

  on<T>(eventType: string, handler: TypedEventHandler<T>): void {
    this.typedHandlers.set(eventType, handler as TypedEventHandler<unknown>);
    if (this.eventSource) {
      this.attachTypedHandler(eventType, handler as TypedEventHandler<unknown>);
    }
  }

  off(eventType: string): void {
    this.typedHandlers.delete(eventType);
  }

  private attachTypedHandler(eventType: string, handler: TypedEventHandler<unknown>): void {
    this.eventSource?.addEventListener(eventType, (event: MessageEvent) => {
      try {
        const data: unknown = JSON.parse(String(event.data));
        handler(data);
      } catch (error) {
        console.error(`Error parsing ${eventType} event:`, error);
      }
    });
  }

  private setStatus(status: SSEStatus): void {
    this.status = status;
    this.config.onStatusChange?.(status);
  }

  private async verifyAuthOrRedirect(): Promise<boolean> {
    const creds = localStorage.getItem('auth_credentials');
    if (!creds) {
      toast.error('Session expired. Please log in again.');
      clearAuthState();
      return true;
    }

    try {
      const response = await fetch(`${API_BASE_URL}/api/streams`, {
        headers: { 'Authorization': `Basic ${creds}` },
      });
      if (response.status === 401) {
        toast.error('Session expired. Please log in again.');
        clearAuthState();
        return true;
      }
    } catch {
      // Network error, not auth issue
    }

    return false;
  }

  private scheduleReconnect(): void {
    if (this.reconnectTimeout) {
      window.clearTimeout(this.reconnectTimeout);
    }

    const currentDelay = this.reconnectDelay;
    console.log(`SSE reconnecting in ${currentDelay / 1000} seconds...`);

    this.reconnectTimeout = window.setTimeout(() => {
      this.connect();
      this.reconnectDelay = Math.min(currentDelay * 2, MAX_RECONNECT_DELAY);
    }, currentDelay);
  }
}
