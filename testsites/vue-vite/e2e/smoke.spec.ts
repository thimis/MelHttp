import type { Locator } from '@playwright/test'
import { expect, test } from './fixtures'

async function expectImageLoaded(img: Locator) {
  await expect(img).toBeVisible()
  await expect
    .poll(() => img.evaluate((el: HTMLImageElement) => el.complete && el.naturalWidth))
    .toBeGreaterThan(0)
}

test('home renders', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('heading', { level: 1, name: 'Vue on MelHttp' })).toBeVisible()
  await expect(page.getByRole('list', { name: 'Malbolge instructions' }).getByRole('listitem')).toHaveCount(8)
})

test('counter increments', async ({ page }) => {
  await page.goto('/')
  const count = page.getByTestId('count')
  await expect(count).toHaveText('0')
  await page.getByRole('button', { name: 'Increment' }).click()
  await page.getByRole('button', { name: 'Increment' }).click()
  await expect(count).toHaveText('2')
  await page.getByRole('button', { name: 'Decrement' }).click()
  await expect(count).toHaveText('1')
})

test('nav link routes to /about without a full reload', async ({ page }) => {
  await page.goto('/')
  await page.evaluate(() => ((window as unknown as { __spaMarker: string }).__spaMarker = 'kept'))
  await page.getByRole('navigation', { name: 'Main' }).getByRole('link', { name: 'About' }).click()
  await expect(page).toHaveURL(/\/about$/)
  await expect(page.getByRole('heading', { level: 1, name: 'About' })).toBeVisible()
  expect(await page.evaluate(() => (window as unknown as { __spaMarker?: string }).__spaMarker)).toBe('kept')
  await page.getByRole('navigation', { name: 'Main' }).getByRole('link', { name: 'Home' }).click()
  await expect(page).toHaveURL(/\/$/)
})

test('direct load of /about works (history fallback)', async ({ page }) => {
  await page.goto('/about')
  await expect(page.getByRole('heading', { level: 1, name: 'About' })).toBeVisible()
  await page.reload()
  await expect(page.getByRole('heading', { level: 1, name: 'About' })).toBeVisible()
})

test('unknown path shows not found', async ({ page }) => {
  await page.goto('/nope/nothing-here')
  await expect(page.getByRole('heading', { level: 1, name: 'Page not found' })).toBeVisible()
})

test('images load', async ({ page }) => {
  await page.goto('/')
  await expectImageLoaded(page.getByTestId('hero-image'))
  await expectImageLoaded(page.getByRole('img', { name: 'Vue logo' }))
})
