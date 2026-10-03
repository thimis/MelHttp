import { test as base, expect } from '@playwright/test';

interface Fixtures {
  /** Patterns of console errors tolerated in the current test (empty by default). */
  consoleAllowList: RegExp[];
  /**
   * Allow-list a console error by pattern for the current test only (e.g. the browser's own
   * "Failed to load resource: 404" line for an endpoint that may legitimately be absent).
   */
  allowConsoleError: (pattern: RegExp) => void;
  failOnConsoleErrors: void;
}

/**
 * Shared fixture: every test fails if the page logs a console error or throws an uncaught
 * exception (pageerror) at any point during the test.
 */
export const test = base.extend<Fixtures>({
  consoleAllowList: async ({}, use) => {
    await use([]);
  },
  allowConsoleError: async ({ consoleAllowList }, use) => {
    await use((pattern) => {
      consoleAllowList.push(pattern);
    });
  },
  failOnConsoleErrors: [
    async ({ page, consoleAllowList }, use) => {
      const problems: string[] = [];
      page.on('console', (msg) => {
        if (msg.type() !== 'error') return;
        const loc = msg.location();
        const line = `console.error: ${msg.text()}${loc.url ? ` (${loc.url}:${loc.lineNumber})` : ''}`;
        if (!consoleAllowList.some((re) => re.test(line))) problems.push(line);
      });
      page.on('pageerror', (err) => problems.push(`pageerror: ${err.message}`));
      await use();
      expect(problems, 'console errors / page errors during the test').toEqual([]);
    },
    { auto: true },
  ],
});

export { expect };
