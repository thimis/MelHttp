import type { Locator, Page } from '@playwright/test';
import { expect, test } from './fixtures';

const rows = (page: Page) => page.locator('table[mat-table] tbody tr[mat-row]');
const rangeLabel = (page: Page) => page.locator('.mat-mdc-paginator-range-label');

async function columnTexts(page: Page, column: string): Promise<string[]> {
  const cells = page.locator(`table[mat-table] tbody td.mat-column-${column}`);
  return (await cells.allTextContents()).map((t) => t.trim());
}

function nav(page: Page): Locator {
  return page.getByRole('navigation', { name: 'Main' });
}

test.describe('shell & routing', () => {
  test('root redirects to /dashboard', async ({ page }) => {
    await page.goto('/');
    await expect(page).toHaveURL(/\/dashboard$/);
    await expect(page.getByRole('heading', { level: 1, name: 'Dashboard' })).toBeVisible();
  });

  test('nav links route client-side without a full reload', async ({ page }) => {
    await page.goto('/dashboard');
    await expect(page.getByRole('heading', { level: 1, name: 'Dashboard' })).toBeVisible();
    await page.evaluate(() => ((window as unknown as { __spaMarker: string }).__spaMarker = 'still-here'));

    const targets: Array<[string, RegExp, string]> = [
      ['Forms', /\/forms$/, 'Forms'],
      ['Theming', /\/theming$/, 'Theming'],
      ['Malbolge corner', /\/malbolge$/, 'Malbolge corner'],
      ['Dashboard', /\/dashboard$/, 'Dashboard'],
    ];
    for (const [link, url, heading] of targets) {
      await nav(page).getByRole('link', { name: link }).click();
      await expect(page).toHaveURL(url);
      await expect(page.getByRole('heading', { level: 1, name: heading })).toBeVisible();
    }
    const marker = await page.evaluate(() => (window as unknown as { __spaMarker?: string }).__spaMarker);
    expect(marker).toBe('still-here');
  });

  test('deep link to /forms survives a reload', async ({ page }) => {
    await page.goto('/forms');
    await expect(page.getByRole('heading', { level: 1, name: 'Forms' })).toBeVisible();
    await page.reload();
    await expect(page).toHaveURL(/\/forms$/);
    await expect(page.getByRole('heading', { level: 1, name: 'Forms' })).toBeVisible();
    await expect(page.getByLabel('Email')).toBeVisible();
  });

  test('unknown routes render the not-found page', async ({ page }) => {
    await page.goto('/no/such/page');
    await expect(page.getByRole('heading', { name: 'Page not found' })).toBeVisible();
    await page.getByRole('link', { name: 'Back to the dashboard' }).click();
    await expect(page).toHaveURL(/\/dashboard$/);
  });

  test('theme toggle switches and persists the theme', async ({ page }) => {
    await page.goto('/dashboard');
    const body = page.locator('body');
    await expect(body).toHaveClass(/(light|dark)-theme/);
    const wasDark = ((await body.getAttribute('class')) ?? '').includes('dark-theme');
    const bgBefore = await body.evaluate((el) => getComputedStyle(el).backgroundColor);

    await page.getByRole('button', { name: 'Toggle theme' }).click();
    await expect(body).toHaveClass(wasDark ? /light-theme/ : /dark-theme/);
    await expect
      .poll(() => body.evaluate((el) => getComputedStyle(el).backgroundColor))
      .not.toBe(bgBefore);

    await page.reload();
    await expect(body).toHaveClass(wasDark ? /light-theme/ : /dark-theme/);

    await page.getByRole('button', { name: 'Toggle theme' }).click();
    await expect(body).toHaveClass(wasDark ? /dark-theme/ : /light-theme/);
  });
});

test.describe('dashboard', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/dashboard');
    await expect(rows(page)).toHaveCount(10);
  });

  test('deep link shows 4 stat cards and the table', async ({ page }) => {
    await expect(page.locator('app-stat-card')).toHaveCount(4);
    await expect(page.locator('app-stat-card').first()).toContainText('Sites served');
    await expect(page.getByRole('table')).toBeVisible();
    await expect(rangeLabel(page)).toHaveText(/1\s*[–-]\s*10 of 50/);
  });

  test('sorting a column changes the row order', async ({ page }) => {
    const before = await columnTexts(page, 'name');
    await page.locator('th.mat-column-name').click();
    await expect.poll(() => columnTexts(page, 'name')).not.toEqual(before);
    const asc = await columnTexts(page, 'name');
    expect(asc).toEqual([...asc].sort((a, b) => a.localeCompare(b)));

    await page.locator('th.mat-column-name').click();
    await expect.poll(() => columnTexts(page, 'name')).not.toEqual(asc);
    const desc = await columnTexts(page, 'name');
    expect(desc).toEqual([...desc].sort((a, b) => b.localeCompare(a)));
  });

  test('filter reduces the rows', async ({ page }) => {
    await page.getByLabel('Filter').fill('Vue');
    await expect(rangeLabel(page)).not.toHaveText(/of 50$/);
    const frameworks = await columnTexts(page, 'framework');
    expect(frameworks.length).toBeGreaterThan(0);
    expect(frameworks.length).toBeLessThanOrEqual(10);
    for (const f of frameworks) expect(f).toBe('Vue');

    await page.getByLabel('Filter').fill('zzz-no-match');
    await expect(rows(page)).toHaveCount(0);
    await expect(page.getByText('No sites match the filter.')).toBeVisible();
  });

  test('paginator moves between pages', async ({ page }) => {
    const firstIds = await columnTexts(page, 'id');
    expect(firstIds[0]).toBe('1');
    await page.getByRole('button', { name: 'Next page' }).click();
    await expect(rangeLabel(page)).toHaveText(/11\s*[–-]\s*20 of 50/);
    await expect.poll(() => columnTexts(page, 'id')).toContain('11');
    await page.getByRole('button', { name: 'Last page' }).click();
    await expect(rangeLabel(page)).toHaveText(/41\s*[–-]\s*50 of 50/);
  });
});

test.describe('forms', () => {
  test('stepper validates, advances and submits', async ({ page }) => {
    await page.goto('/forms');
    const name = page.getByLabel('Full name');
    const email = page.getByLabel('Email');
    const trits = page.getByLabel('Lucky trits');

    await name.fill('Ben Olmstead');
    await email.fill('not-an-email');
    await email.blur();
    await expect(page.getByText('Please enter a valid email address')).toBeVisible();

    // Linear stepper: Next must not advance while the step is invalid.
    await page.getByRole('button', { name: 'Next' }).click();
    await expect(page.getByRole('tab', { name: /Personal info/ })).toHaveAttribute('aria-selected', 'true');

    await email.fill('ben@example.com');
    await expect(page.getByText('Please enter a valid email address')).toBeHidden();

    // Custom MatFormFieldControl drops non-trit characters.
    await trits.pressSequentially('0a1-2 9');
    await expect(trits).toHaveValue('012');

    await page.getByRole('button', { name: 'Next' }).click();
    await expect(page.getByRole('tab', { name: /Preferences/ })).toHaveAttribute('aria-selected', 'true');
    await expect(page.getByLabel('Launch date')).toBeVisible();

    // Add a chip and toggle the slide toggle.
    const tagInput = page.getByPlaceholder('Add tag…');
    await tagInput.fill('melhttp');
    await tagInput.press('Enter');
    await expect(page.locator('mat-chip-row', { hasText: 'melhttp' })).toBeVisible();

    await page.getByRole('button', { name: 'Next' }).click();
    await expect(page.getByRole('tab', { name: /Review/ })).toHaveAttribute('aria-selected', 'true');
    await expect(page.getByTestId('summary-name')).toHaveText('Ben Olmstead');
    await expect(page.getByTestId('summary-email')).toHaveText('ben@example.com');

    await page.getByRole('button', { name: 'Submit' }).click();
    await expect(page.locator('mat-snack-bar-container')).toContainText('Thanks Ben Olmstead!');
  });
});

test.describe('theming', () => {
  test('dialog opens and closes', async ({ page }) => {
    await page.goto('/theming');
    await page.getByRole('button', { name: 'Open dialog' }).click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();
    await expect(dialog).toContainText('The eight Malbolge instructions');
    await dialog.getByRole('button', { name: 'Close' }).click();
    await expect(dialog).toBeHidden();
    await expect(page.getByTestId('dialog-result')).toHaveText('Last dialog result: dismissed');
  });

  test('snackbar shows', async ({ page }) => {
    await page.goto('/theming');
    await page.getByRole('button', { name: 'Show snackbar', exact: true }).click();
    await expect(page.locator('mat-snack-bar-container')).toContainText('printed one Malbolge step');
  });
});

test.describe('malbolge corner', () => {
  test('renders without console errors', async ({ page }) => {
    await page.goto('/malbolge');
    await expect(page.getByRole('heading', { level: 1, name: 'Malbolge corner' })).toBeVisible();
    await expect(page.getByText('every byte of this page was printed by a Malbolge program')).toBeVisible();
    // Headers either load (any server) or fail gracefully; never stay stuck or throw.
    const card = page.getByTestId('headers-card');
    await expect(card.locator('[data-header="X-Malbolge-Steps"]').or(card.getByText('Could not read'))).toBeVisible();
  });

  test('source viewer shows source or a friendly message', async ({ page, allowConsoleError }) => {
    // If /_source is not exposed, the browser itself logs "Failed to load resource" for the 404.
    allowConsoleError(/Failed to load resource.*\/_source\//);
    await page.goto('/malbolge');
    await page.getByRole('button', { name: 'Fetch source' }).click();
    await expect(page.getByTestId('source-pre').or(page.getByTestId('source-unavailable'))).toBeVisible();
  });
});
