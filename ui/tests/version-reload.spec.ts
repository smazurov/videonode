import { test, expect, type Page } from '@playwright/test';

// Drives the new-version watcher (src/lib/versionWatch.ts) end to end. /api/health
// is fully mocked so the test owns the reported build version: it boots at
// test-v1, then flips to test-v2 to simulate a redeploy. Auth is seeded into
// localStorage so the protected shell (which wires the watcher) mounts without a
// login round-trip. A sessionStorage load-counter survives reloads, giving a
// deterministic reload signal without racing on navigation events.

const BANNER = 'A new version is available';

let reportedVersion = 'test-v1';

async function boot(page: Page): Promise<void> {
  await page.addInitScript(() => {
    // btoa('videonode:videonode'); inlined because addInitScript runs in the page.
    localStorage.setItem('auth_credentials', 'dmlkZW9ub2RlOnZpZGVvbm9kZQ==');
    localStorage.setItem(
      'auth-storage',
      JSON.stringify({ state: { user: { username: 'videonode', isAuthenticated: true } }, version: 0 }),
    );
    const n = Number(sessionStorage.getItem('__loads') ?? '0') + 1;
    sessionStorage.setItem('__loads', String(n));
  });

  await page.route('**/api/health', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ status: 'ok', message: 'API is healthy', version: reportedVersion }),
    }),
  );

  await page.goto('/streams');
  await expect(page).toHaveURL(/\/streams/);
  // First check seeds the baseline (firstSeenVersion = test-v1).
  await fireVersionCheck(page);
}

const loadCount = (page: Page): Promise<number> =>
  page.evaluate(() => Number(sessionStorage.getItem('__loads') ?? '0'));

// The watcher checks on tab focus; dispatching visibilitychange drives it without
// waiting on an SSE reconnect.
const fireVersionCheck = (page: Page): Promise<void> =>
  page.evaluate(() => document.dispatchEvent(new Event('visibilitychange')));

test.beforeEach(() => {
  reportedVersion = 'test-v1';
});

test('shows a banner and does not reload while the user is busy editing', async ({ page }) => {
  await boot(page);
  const before = await loadCount(page);

  await page.evaluate(() => {
    const input = document.createElement('input');
    input.id = '__busy';
    document.body.appendChild(input);
    input.focus();
  });

  reportedVersion = 'test-v2';
  await fireVersionCheck(page);

  await expect(page.getByText(BANNER)).toBeVisible();
  expect(await loadCount(page)).toBe(before);
});

test('auto-reloads when idle, then settles without a reload loop', async ({ page }) => {
  await boot(page);
  const before = await loadCount(page);

  reportedVersion = 'test-v2';
  await fireVersionCheck(page);

  // Idle → immediate reload (load counter increments).
  await expect.poll(() => loadCount(page)).toBe(before + 1);
  // Mock still reports test-v2, so the reloaded page reseeds to it: no banner, no loop.
  await expect(page.getByText(BANNER)).toHaveCount(0);
});
