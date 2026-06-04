import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  // Matches the .playwright*/ entry already in .gitignore.
  outputDir: './.playwright-results',
  timeout: 30_000,
  use: {
    baseURL: 'http://localhost:5199',
    viewport: { width: 1280, height: 720 },
    actionTimeout: 5_000,
  },
  projects: [
    { name: 'chromium', use: { browserName: 'chromium' } },
  ],
  webServer: {
    command: 'pnpm dev --port 5199',
    port: 5199,
    reuseExistingServer: true,
  },
});
