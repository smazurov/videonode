import createClient, { type Middleware } from "openapi-fetch";
import { toast } from 'react-hot-toast';
import { clearAuthState } from '../hooks/useAuthStore';
import { getAuthCredentials } from './auth';
import type { paths } from "./api.generated";

export const API_BASE_URL = window.location.origin;

const SESSION_EXPIRED_MSG = 'Session expired. Please log in again.';

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
    this.name = "ApiError";
  }
}

// unwrap throws if the openapi-fetch result has an error, otherwise returns
// the data. Centralizes the if (error) throw idiom used at every call site.
export function unwrap<TData, TError extends { detail?: string } | undefined>(
  result: { data?: TData; error?: TError },
  fallbackMsg: string,
): TData {
  if (result.error) {
    throw new Error(result.error.detail ?? fallbackMsg);
  }
  return result.data as TData;
}

const authMiddleware: Middleware = {
  async onRequest({ request }) {
    const credentials = getAuthCredentials();
    if (credentials) {
      request.headers.set('Authorization', `Basic ${credentials}`);
    }
    return request;
  },
  async onResponse({ response }) {
    if (response.status === 401) {
      toast.error(SESSION_EXPIRED_MSG);
      clearAuthState();
    }
    return response;
  },
};

export const api = createClient<paths>({ baseUrl: API_BASE_URL });
api.use(authMiddleware);

// Path-scoped typed clients. They share the underlying authenticated client
// (same fetch + middleware) but constrain the path generic, so call sites for
// a given entity only see the relevant routes in autocomplete.
type Pick<P extends keyof paths> = { [K in P]: paths[K] };

type SourcePath = Extract<keyof paths, `/api/sources${string}`>;
type ComposerPath = Extract<keyof paths, `/api/composers${string}`>;
type StreamPath = Extract<keyof paths, `/api/streams${string}` | `/api/v2/streams${string}`>;

export const apiSources = api as unknown as ReturnType<typeof createClient<Pick<SourcePath>>>;
export const apiComposers = api as unknown as ReturnType<typeof createClient<Pick<ComposerPath>>>;
export const apiStreams = api as unknown as ReturnType<typeof createClient<Pick<StreamPath>>>;

export function buildStreamURL(partialUrl: string | undefined, protocol: 'http' | 'rtsp' | 'srt' = 'http'): string | undefined {
  if (!partialUrl) return undefined;

  if (partialUrl.startsWith(':')) {
    const fullUrl = `${protocol}://${window.location.hostname}${partialUrl}`;
    if (protocol === 'srt') {
      return `${fullUrl}&latency=50000`;
    }
    return fullUrl;
  }

  return partialUrl;
}

export async function testAuth(username: string, password: string): Promise<boolean> {
  const credentials = btoa(`${username}:${password}`);

  try {
    const response = await fetch(`${API_BASE_URL}/api/streams`, {
      headers: {
        'Authorization': `Basic ${credentials}`,
        'Content-Type': 'application/json',
      },
    });

    return response.ok;
  } catch (error) {
    console.error("Auth test failed:", error);
    return false;
  }
}

export async function webrtcSignaling(streamId: string, offer: string, signal?: AbortSignal): Promise<string> {
  const credentials = getAuthCredentials();
  const headers: HeadersInit = {
    'Content-Type': 'application/sdp',
  };

  if (credentials) {
    headers['Authorization'] = `Basic ${credentials}`;
  }

  const response = await fetch(`${API_BASE_URL}/api/webrtc?stream=${encodeURIComponent(streamId)}`, {
    method: 'POST',
    headers,
    body: offer,
    signal: signal ?? null,
  });

  if (!response.ok) {
    if (response.status === 401) {
      toast.error(SESSION_EXPIRED_MSG);
      clearAuthState();
    }
    throw new ApiError(response.status, `WebRTC signaling failed: ${response.statusText}`);
  }

  return response.text();
}

// SSE type helpers — extract event→data map from generated path types
type GetContent<P extends keyof paths> =
  paths[P] extends { get: { responses: { 200: { content: infer C } } } } ? C : never;

type SSEPath = {
  [P in keyof paths]: GetContent<P> extends { "text/event-stream": unknown }
    ? P
    : never;
}[keyof paths];

type SSEStream<P extends SSEPath> = GetContent<P> extends {
  "text/event-stream": infer S;
}
  ? S
  : never;

type SSEEvent<P extends SSEPath> = SSEStream<P> extends (infer E)[] ? E : never;

type SSEEventMap<P extends SSEPath> = {
  [E in SSEEvent<P> as E extends { event: infer N extends string }
    ? N
    : never]: E extends { data: infer D } ? D : never;
};

export type SSEStatus =
  | "connecting"
  | "connected"
  | "disconnected"
  | "reconnecting";

const INITIAL_RECONNECT_DELAY = 5000;
const MAX_RECONNECT_DELAY = 60000;
export interface SSEClientConfig<P extends SSEPath> {
  endpoint: P;
  onStatusChange?: (status: SSEStatus) => void;
  onConnect?: () => void;
  onError?: (willReconnect: boolean) => void;
}

type MessageHandler = (event: MessageEvent) => void;
type TypedEventHandler<T> = (data: T) => void;

export class SSEClient<P extends SSEPath> {
  private eventSource: EventSource | null = null;
  private reconnectTimeout: number | null = null;
  private reconnectDelay = INITIAL_RECONNECT_DELAY;
  private messageHandler: MessageHandler | null = null;
  private readonly typedHandlers: Map<string, TypedEventHandler<unknown>> =
    new Map();
  private status: SSEStatus = "disconnected";

  constructor(private readonly config: SSEClientConfig<P>) {}

  connect(): void {
    if (this.eventSource) return;

    const credentials = getAuthCredentials();
    if (!credentials) {
      this.setStatus('disconnected');
      return;
    }

    this.setStatus("connecting");

    const sseUrl = `${API_BASE_URL}${this.config.endpoint}?auth=${encodeURIComponent(credentials)}`;
    this.eventSource = new EventSource(sseUrl);

    this.eventSource.onopen = () => {
      this.reconnectDelay = INITIAL_RECONNECT_DELAY;
      this.setStatus("connected");
      this.config.onConnect?.();
    };

    if (this.messageHandler) {
      this.eventSource.onmessage = this.messageHandler;
    }

    for (const [eventType, handler] of this.typedHandlers) {
      this.attachTypedHandler(eventType, handler);
    }

    this.eventSource.onerror = async () => {
      this.eventSource?.close();
      this.eventSource = null;

      const authFailed = await this.verifyAuthOrRedirect();
      if (authFailed) {
        this.setStatus('disconnected');
        this.config.onError?.(false);
        return;
      }

      // Sitting in the backoff wait reads as 'disconnected'. The active
      // EventSource attempt in connect() is the only thing that surfaces as
      // 'connecting' — so "reconnecting" stays honest, shown only while a
      // connection is genuinely being attempted, not during the idle wait.
      this.setStatus('disconnected');
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
    this.setStatus("disconnected");
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

  on<K extends keyof SSEEventMap<P> & string>(
    eventType: K,
    handler: TypedEventHandler<SSEEventMap<P>[K]>,
  ): void {
    this.typedHandlers.set(
      eventType,
      handler as TypedEventHandler<unknown>,
    );
    if (this.eventSource) {
      this.attachTypedHandler(
        eventType,
        handler as TypedEventHandler<unknown>,
      );
    }
  }

  off(eventType: keyof SSEEventMap<P> & string): void {
    this.typedHandlers.delete(eventType);
  }

  private attachTypedHandler(
    eventType: string,
    handler: TypedEventHandler<unknown>,
  ): void {
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
    if (!getAuthCredentials()) {
      toast.error(SESSION_EXPIRED_MSG);
      clearAuthState();
      return true;
    }
    // authMiddleware.onResponse handles the 401 toast + clearAuthState; we
    // just need to know whether the call succeeded enough to continue
    // reconnect attempts. openapi-fetch rejects (not returns) on a transport
    // failure, so a downed backend throws here — swallow it and keep
    // reconnecting rather than letting the rejection escape this async
    // onerror handler as an unhandled promise rejection.
    try {
      const { response } = await api.GET('/api/streams');
      return response?.status === 401;
    } catch {
      return false;
    }
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
