import { defineConfig, devices } from '@playwright/test'

/**
 * Smoke tests for the built site. The server is NOT started here: the harness (or you) serves
 * dist/ with SPA fallback and points BASE_URL at it, e.g.
 *   npx vite preview --port 4173
 *   BASE_URL=http://localhost:4173 npx playwright test
 */
export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: 0,
  reporter: 'line',
  timeout: 30_000,
  use: {
    baseURL: process.env.BASE_URL ?? 'http://localhost:4173',
    trace: 'retain-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
})
