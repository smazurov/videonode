import { api } from './api';
import { ApiError } from './api_fetch';

// WHEP (WebRTC-HTTP Egress Protocol) signaling, driven through the typed client.
// The offer/answer bodies are application/sdp and the answer is plain text, so
// bodySerializer/parseAs bypass the client's default JSON handling.

// whepConnect POSTs the SDP offer to /whep/{stream} and returns the SDP answer
// plus the session id parsed from the 201 Location header (used for teardown).
export async function whepConnect(
  streamId: string,
  offer: string,
  signal?: AbortSignal,
): Promise<{ answer: string; sessionId: string | null }> {
  const { data, error, response } = await api.POST('/whep/{stream}', {
    params: { path: { stream: streamId } },
    body: offer as unknown as never,
    bodySerializer: (body: unknown) => body as string,
    headers: { 'Content-Type': 'application/sdp' },
    parseAs: 'text',
    signal: signal ?? null,
  });
  if (error !== undefined) {
    throw new ApiError(response.status, `WHEP signaling failed: ${response.statusText}`);
  }
  const location = response.headers.get('Location');
  const sessionId = location ? location.split('/').pop() ?? null : null;
  return { answer: data as unknown as string, sessionId };
}

// whepTeardown deletes a WHEP session for prompt server-side cleanup. Best
// effort: a failed teardown is logged, never thrown (the peer connection is
// closed regardless).
export async function whepTeardown(
  streamId: string,
  sessionId: string,
  signal?: AbortSignal,
): Promise<void> {
  try {
    await api.DELETE('/whep/{stream}/{session}', {
      params: { path: { stream: streamId, session: sessionId } },
      signal: signal ?? null,
    });
  } catch (error) {
    console.warn(`WHEP teardown failed for ${streamId}/${sessionId}:`, error);
  }
}
