// Allowlisted raw-request layer. This is the single sanctioned home for HTTP
// calls that bypass the typed `api` client (src/lib/api.ts). The no-raw-fetch
// ESLint rule is disabled only for this file; everything else must go through
// the typed client.

export const API_BASE_URL = window.location.origin;

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

