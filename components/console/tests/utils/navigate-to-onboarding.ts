import { Page, expect } from '@playwright/test'

export async function navigateToOnboarding(page: Page) {
  await page.goto('/onboarding')
  await page.waitForLoadState('networkidle')
  // Wait for onboarding page to be ready
  await expect(page.getByTestId('onboarding-dialog-title')).toBeVisible({
    timeout: 10000
  })
  // Click on "Let's go" button
  await page.getByRole('button', { name: /Let's go|Vamos lá/i }).click()
}
