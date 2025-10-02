import { Page, expect } from '@playwright/test'

export async function navigateToOnboarding(page: Page) {
  await page.goto('/onboarding')
  await page.waitForLoadState('networkidle')
  // Wait for onboarding page to be ready
  await expect(page.getByTestId('title').or(page.locator('h1'))).toBeVisible({
    timeout: 10000
  })
}
