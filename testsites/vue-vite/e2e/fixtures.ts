import { test as base, expect } from '@playwright/test'

/**
 * Shared fixture: every test fails if the page logs a console error or throws an uncaught
 * exception (pageerror) at any point during the test.
 */
export const test = base.extend<{ failOnConsoleErrors: void }>({
  failOnConsoleErrors: [
    async ({ page }, use) => {
      const problems: string[] = []
      page.on('console', (msg) => {
        if (msg.type() !== 'error') return
        const loc = msg.location()
        problems.push(`console.error: ${msg.text()}${loc.url ? ` (${loc.url}:${loc.lineNumber})` : ''}`)
      })
      page.on('pageerror', (err) => problems.push(`pageerror: ${err.message}`))
      await use()
      expect(problems, 'console errors / page errors during the test').toEqual([])
    },
    { auto: true },
  ],
})

export { expect }
