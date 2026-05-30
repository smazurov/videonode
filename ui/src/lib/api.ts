import createClient, { type Middleware } from "openapi-fetch";
import { toast } from 'react-hot-toast';
import { clearAuthState } from '../hooks/useAuthStore';
import { getAuthCredentials } from './auth';
import type { paths } from "./api.generated";

export const API_BASE_URL = window.location.origin;

const SESSION_EXPIRED_MSG = 'Session expired. Please log in again.';

// Default ceiling for any single request. Without it, a black-holed socket
// (rig powered off, network partition) leaves fetches pending until the OS
// gives up — 60–120s — during which interval pollers stack doomed requests.
const DEFAULT_FETCH_TIMEOUT_MS = 8000;

// Combine an optional caller signal with an internal one. AbortSignal.any is
// widely available, but fall back to manual forwarding so an older engine
// still honours both the caller's abort and the timeout.
function anySignal(signals: AbortSignal[]): AbortSignal {
  if (typeof AbortSignal.any === 'function') return AbortSignal.any(signals);
  const controller = new AbortController();
  for (const s of signals) {
    if (s.aborted) {
      controller.abort(s.reason);
      break;
    }
    s.addEventListener('abort', () => controller.abort(s.reason), { once: true });
  }
  return controller.signal;
}

// fetchWithTimeout wraps the global fetch so every request rejects after
// DEFAULT_FETCH_TIMEOUT_MS instead of hanging on a dead host. Passed to
// createClient below, so all typed api.* calls inherit it; a caller-supplied
// signal (e.g. unmount AbortControllers) is combined, not overridden.
export function fetchWithTimeout(
  input: RequestInfo | URL,
  init?: RequestInit,
): Promise<Response> {
  const timeout = new AbortController();
  const timer = setTimeout(() => timeout.abort(), DEFAULT_FETCH_TIMEOUT_MS);
  const signal = init?.signal
    ? anySignal([init.signal, timeout.signal])
    : timeout.signal;
  return fetch(input, { ...init, signal }).finally(() => clearTimeout(timer));
}

// probeHealth pings the unauthenticated /api/health endpoint with a short
// timeout. Returns true only on a 2xx; any transport failure or timeout is
// false. The reconnect gate uses this to avoid reopening an EventSource
// against a host that's still down.
export async function probeHealth(timeoutMs = 5000): Promise<boolean> {
  try {
    const response = await fetch(`${API_BASE_URL}/api/health`, {
      signal: AbortSignal.timeout(timeoutMs),
    });
    return response.ok;
  } catch {
    return false;
  }
}

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

export const api = createClient<paths>({ baseUrl: API_BASE_URL, fetch: fetchWithTimeout });
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
    const response = await fetchWithTimeout(`${API_BASE_URL}/api/streams`, {
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

  const response = await fetchWithTimeout(`${API_BASE_URL}/api/webrtc?stream=${encodeURIComponent(streamId)}`, {
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
// Server heartbeats every 15s (internal/api/events.go). If 20s pass with no
// event of any kind, one beat was genuinely missed (15s + grace for jitter):
// treat the connection as dead now rather than waiting out the OS TCP timeout.
const HEARTBEAT_TIMEOUT_MS = 20000;
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
  private watchdogTimer: number | null = null;
  // Bumped by disconnect(). The health-gated reconnect awaits between
  // scheduling and reopening; capturing the generation lets an in-flight
  // attempt bail if disconnect() ran during its await, preserving the original
  // "disconnect cancels any pending reconnect" guarantee.
  private generation = 0;
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
      this.resetWatchdog();
      this.config.onConnect?.();
    };

    if (this.messageHandler) {
      this.eventSource.onmessage = this.messageHandler;
    }

    for (const [eventType, handler] of this.typedHandlers) {
      this.attachTypedHandler(eventType, handler);
    }

    // Internal heartbeat listener: the server's 15s keep-alive isn't wired to
    // any external handler, but it's still proof of life — let it pet the
    // watchdog so an otherwise-idle (no entity events) connection stays up.
    this.eventSource.addEventListener('heartbeat', () => this.resetWatchdog());

    this.eventSource.onerror = () => {
      void this.handleDisconnect();
    };
  }

  // handleDisconnect tears down the current EventSource and decides what next:
  // a 401 stops retrying; anything else enters the health-gated reconnect
  // loop. Shared by onerror and the heartbeat watchdog.
  private async handleDisconnect(): Promise<void> {
    this.clearWatchdog();
    this.eventSource?.close();
    this.eventSource = null;

    const gen = this.generation;
    const authFailed = await this.verifyAuthOrRedirect();
    if (gen !== this.generation) return; // disconnect() ran during the await
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
  }

  // resetWatchdog (re)arms the liveness timer on any server activity. If it
  // fires, a heartbeat was missed: the socket is likely black-holed (no
  // onerror yet), so force the disconnect path.
  private resetWatchdog(): void {
    this.clearWatchdog();
    this.watchdogTimer = window.setTimeout(() => {
      void this.handleDisconnect();
    }, HEARTBEAT_TIMEOUT_MS);
  }

  private clearWatchdog(): void {
    if (this.watchdogTimer != null) {
      window.clearTimeout(this.watchdogTimer);
      this.watchdogTimer = null;
    }
  }

  disconnect(): void {
    this.generation++;
    this.clearWatchdog();
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
      this.resetWatchdog();
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

  // scheduleReconnect waits out the backoff, then probes /api/health before
  // touching the EventSource. A dead host never gets a reconnect attempt — we
  // re-probe on a growing backoff and stay 'disconnected' (→ offline) until
  // health actually returns, then connect() opens the stream.
  private scheduleReconnect(): void {
    if (this.reconnectTimeout) {
      window.clearTimeout(this.reconnectTimeout);
    }

    const currentDelay = this.reconnectDelay;
    const gen = this.generation;
    console.log(`SSE reconnecting in ${currentDelay / 1000} seconds...`);

    this.reconnectTimeout = window.setTimeout(() => {
      void this.attemptReconnect(currentDelay, gen);
    }, currentDelay);
  }

  private async attemptReconnect(currentDelay: number, gen: number): Promise<void> {
    if (gen !== this.generation) return; // disconnected before this fired
    // An attempt is genuinely in flight now — surface as 'connecting'
    // (→ reconnecting) while the probe runs.
    this.setStatus('connecting');
    const healthy = await probeHealth();
    if (gen !== this.generation) return; // disconnected during the probe
    if (!healthy) {
      // Still down: grow the backoff, drop back to the idle (offline) wait,
      // and try again. Never open an EventSource against a dead host.
      this.reconnectDelay = Math.min(currentDelay * 2, MAX_RECONNECT_DELAY);
      this.setStatus('disconnected');
      this.scheduleReconnect();
      return;
    }
    this.reconnectDelay = Math.min(currentDelay * 2, MAX_RECONNECT_DELAY);
    this.connect();
  }
}
