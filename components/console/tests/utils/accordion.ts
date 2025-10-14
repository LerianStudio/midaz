import type { Page } from '@playwright/test'

/**
 * Expands an accordion by clicking its trigger
 * @param page - Playwright page object
 * @param testId - The test ID of the accordion element
 */
export async function expandAccordion(page: Page, testId: string) {
  // Wait for the accordion to be visible
  const accordion = page.getByTestId(testId)
  await accordion.waitFor({ state: 'visible', timeout: 5000 })

  // Click the chevron trigger to expand the accordion
  const trigger = accordion.getByTestId('paper-collapsible-trigger')
  await trigger.click()
  await page.waitForTimeout(500)
}
