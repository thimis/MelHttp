import { expect, test } from './fixtures';

/**
 * The whole app over the Malbolge transport. Runs only against a server started with
 * -obfuscate-inject (MELHTTP_TRANSPORT=1); playwright.config.ts selects it.
 */
test('the app works when every file travels as Malbolge', async ({ page }) => {
  test.setTimeout(180_000);
  await page.goto('/dashboard');
  await page.evaluate(() => navigator.serviceWorker.ready);
  await page.reload(); // now controlled by the worker
  await expect(page.locator('html')).toHaveAttribute('data-malbolge-transport', 'active');
  await expect(page.getByRole('heading', { level: 1, name: 'Dashboard' })).toBeVisible();

  // The page and every app script were decoded from Malbolge by the worker
  // (the transport's own files under /_melhttp/ travel plain: they are the decoder).
  const files = await page.evaluate(async () => {
    const scripts = Array.from(document.scripts, (s) => s.src).filter((src) => src && !src.includes('/_melhttp/'));
    const urls = ['/dashboard', ...scripts];
    return Promise.all(urls.map(async (url) => {
      const r = await fetch(url);
      return { url, decoded: r.headers.get('X-Malbolge-Decoded'), size: (await r.arrayBuffer()).byteLength };
    }));
  });
  expect(files.length).toBeGreaterThan(1);
  for (const f of files) {
    expect(f.decoded, f.url).toBe('service-worker');
    expect(f.size, f.url).toBeGreaterThan(0);
  }

  // The server added the transport script once, inside <head>.
  const html = await page.evaluate(async () => (await fetch('/dashboard')).text());
  expect(html.split('/_melhttp/obfuscate.js').length - 1).toBe(1);
  expect(html.indexOf('/_melhttp/obfuscate.js')).toBeLessThan(html.toLowerCase().indexOf('</head>'));

  // On the wire it is Malbolge (Playwright's request client bypasses the worker).
  const raw = await page.request.get('/dashboard', { headers: { 'X-Malbolge-Accept': 'program' } });
  expect(raw.headers()['x-malbolge-encoding']).toBe('program');
  const wire = await raw.text();
  expect(wire).not.toContain('MelHttp Showcase');
  expect(wire).toMatch(/^[!-~\n]+$/);

  // Deep links (SPA fallback) and client-side routing still work.
  const deep = await page.goto('/forms');
  expect(deep?.headers()['x-malbolge-decoded']).toBe('service-worker');
  await expect(page.getByRole('heading', { level: 1, name: 'Forms' })).toBeVisible();
  await page.getByRole('navigation', { name: 'Main' }).getByRole('link', { name: 'Theming' }).click();
  await expect(page).toHaveURL(/\/theming$/);
  await expect(page.getByRole('heading', { level: 1, name: 'Theming' })).toBeVisible();
});
