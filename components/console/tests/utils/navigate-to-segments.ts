import { Page, expect } from '@playwright/test'

export async function navigateToSegments(page: Page) {
  // Start from home page to ensure clean state
  await page.goto('/', {
    waitUntil: 'domcontentloaded',
    timeout: 60000
  })

  // Wait for page to load
  await page.waitForTimeout(1000)

  // Navigate to segments page
  await page.goto('/segments', {
    waitUntil: 'domcontentloaded',
    timeout: 60000
  })

  // Verify we're actually on the segments page (check URL and heading)
  await page.waitForURL(/.*\/segments/, { timeout: 15000 })

  // Wait for the page heading to be visible - confirms page structure is rendered
  await expect(
    page.getByRole('heading', { name: 'Segments', level: 1 })
  ).toBeVisible({ timeout: 15000 })

  // Wait for the skeleton to disappear - indicates data loading is complete
  await page
    .waitForSelector('[class*="SegmentsSkeleton"], [class*="skeleton"]', {
      state: 'hidden',
      timeout: 15000
    })
    .catch(() => {
      // Skeleton might not appear if data loads quickly
    })

  // Wait for either the new segment button or empty state to confirm page is ready
  await Promise.race([
    page
      .getByRole('button', { name: /new segment/i })
      .waitFor({ state: 'visible', timeout: 15000 }),
    page
      .getByText(/You haven't created any Segments yet/i)
      .waitFor({ state: 'visible', timeout: 15000 })
  ]).catch(() => {
    // If neither appears, continue - page might be loading
  })

  // Additional wait to ensure all JavaScript execution is complete
  await page.waitForTimeout(500)
}
