import { useSyncExternalStore } from 'react';
import { api } from './api';

// versionWatch detects a backend redeploy and reloads the SPA to pick up the
// new bundle. The daemon is a single binary, so a deploy = process restart = an
// SSE disconnect; callers hook checkServerVersion() onto reconnect/visibility
// (see useSSEManager). All detection, the busy guard, and the reload decision
// live here so the rest of the app only renders a banner.
//
// The comparison is server-version vs the first server version this tab saw —
// never the UI build constant vs the Go string — so it's immune to any format
// drift between vite-plugin-version-mark's git describe and the ldflags value.

let firstSeenVersion: string | null = null;
let updatePending = false;
const subscribers = new Set<() => void>();

function emit(): void {
  for (const fn of subscribers) fn();
}

// isBusy reports whether reloading now would interrupt active editing. Single
// source of truth for "don't yank the page"; extend here (e.g. a drag flag) if
// new interactions need protecting.
function isBusy(): boolean {
  const el = document.activeElement as HTMLElement | null;
  if (!el) return false;
  const tag = el.tagName;
  return (
    tag === 'INPUT' ||
    tag === 'TEXTAREA' ||
    tag === 'SELECT' ||
    el.isContentEditable
  );
}

async function fetchServerVersion(timeoutMs = 5000): Promise<string | null> {
  try {
    const { data } = await api.GET('/api/health', {
      cache: 'no-store',
      signal: AbortSignal.timeout(timeoutMs),
    });
    return typeof data?.version === 'string' ? data.version : null;
  } catch {
    return null;
  }
}

// checkServerVersion seeds the baseline on first call, then on any later
// mismatch reloads immediately when idle or raises the update-pending flag when
// the user is busy. Safe to call repeatedly: once flagged it stays flagged, so
// a later idle reconnect/visibility tick completes the deferred reload.
export async function checkServerVersion(): Promise<void> {
  const version = await fetchServerVersion();
  if (version === null) return;

  if (firstSeenVersion === null) {
    firstSeenVersion = version;
    return;
  }

  if (version === firstSeenVersion) return;

  if (isBusy()) {
    if (!updatePending) {
      updatePending = true;
      emit();
    }
    return;
  }

  window.location.reload();
}

export function isUpdatePending(): boolean {
  return updatePending;
}

export function subscribeUpdatePending(fn: () => void): () => void {
  subscribers.add(fn);
  return () => {
    subscribers.delete(fn);
  };
}

export function useVersionUpdatePending(): boolean {
  return useSyncExternalStore(subscribeUpdatePending, isUpdatePending);
}
