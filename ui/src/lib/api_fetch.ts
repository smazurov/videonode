// Allowlisted raw-request layer. This is the single sanctioned home for HTTP
// calls that bypass the typed `api` client (src/lib/api.ts). The no-raw-fetch
// ESLint rule is disabled only for this file; everything else must go through
// the typed client.

export const API_BASE_URL = window.location.origin;

// apiUrl absolutizes a server-relative path ("/api/...") for media elements
// and raw fetches that bypass the typed client.
export const apiUrl = (path?: string): string => (path ? `${API_BASE_URL}${path}` : '');

export const SESSION_EXPIRED_MSG = 'Session expired. Please log in again.';

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
// createClient in api.ts, so all typed api.* calls inherit it; a caller-supplied
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

// fetchJSONRaw GETs a same-origin URL and parses JSON. Used for recording
// artifacts that are static files served outside the typed OpenAPI surface.
// Returns null on any transport/parse failure or non-2xx.
export async function fetchJSONRaw<T>(url: string): Promise<T | null> {
  try {
    const response = await fetchWithTimeout(url);
    if (!response.ok) return null;
    return (await response.json()) as T;
  } catch {
    return null;
  }
}

// fetchTextRaw mirrors fetchJSONRaw for plain-text artifacts (thumbnails.vtt).
export async function fetchTextRaw(url: string): Promise<string | null> {
  try {
    const response = await fetchWithTimeout(url);
    if (!response.ok) return null;
    return await response.text();
  } catch {
    return null;
  }
}

// fetchBitmapRaw GETs an image (sprite sheet) and decodes it off the main
// thread. Returns null on any failure.
export async function fetchBitmapRaw(url: string): Promise<ImageBitmap | null> {
  try {
    const response = await fetchWithTimeout(url);
    if (!response.ok) return null;
    return await createImageBitmap(await response.blob());
  } catch {
    return null;
  }
}

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
    this.name = "ApiError";
  }
}

// testAuth validates an arbitrary username/password before it is stored. It
// must stay raw: the typed client's middleware injects the *stored*
// credentials, so it cannot test credentials the user is still entering.
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

