import { defineConfig, devices } from '@playwright/test';

/**
 * Smoke tests for the built site. The server is NOT started here: the harness (or you) serves
 * dist/angular-showcase/browser with SPA fallback and points BASE_URL at it, e.g.
 *   npx -y serve -s dist/angular-showcase/browser -l 4200
 *   BASE_URL=http://localhost:4200 npx playwright test
 */
// MELHTTP_TRANSPORT=1: the server runs with -obfuscate-inject; run only the transport test.
const transport = process.env['MELHTTP_TRANSPORT'] === '1';

export default defineConfig({
  testDir: './e2e',
  ...(transport ? { testMatch: 'transport.spec.ts' } : { testIgnore: 'transport.spec.ts' }),
  fullyParallel: true,
  forbidOnly: !!process.env['CI'],
  retries: 0,
  reporter: 'line',
  timeout: 30_000,
  expect: { timeout: 10_000 },
  use: {
    baseURL: process.env['BASE_URL'] ?? 'http://localhost:4200',
    trace: 'retain-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
});
