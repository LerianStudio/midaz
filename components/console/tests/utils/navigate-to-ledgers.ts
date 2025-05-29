import { Page, expect } from '@playwright/test'

export async function navigateToLedgers(page: Page) {
  await page.goto('/')
  await page.getByRole('button').first().click()
  await page.getByRole('link', { name: 'Ledgers' }).click()
  await page.waitForURL('/ledgers')
  await expect(page.getByTestId('title')).toBeVisible()
}
