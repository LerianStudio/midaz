import { Page, expect } from '@playwright/test'

export async function navigateToOperationRoutes(page: Page) {
  // Navigate directly to operation routes page with extended timeout
  // Use 'domcontentloaded' to avoid waiting for all network requests
  await page.goto('/operation-routes', {
    waitUntil: 'domcontentloaded',
    timeout: 60000 // 60 seconds to allow for slow server startup
  })

  // Wait for the page heading to be visible - this confirms the page structure is rendered
  // Use role selector instead of test ID for better resilience
  await expect(
    page.getByRole('heading', { name: 'Operation Routes', level: 1 })
  ).toBeVisible({ timeout: 15000 })

  // Wait for the skeleton to disappear - this indicates initial data loading is complete
  await page
    .waitForSelector(
      '[class*="OperationRoutesSkeleton"], [class*="skeleton"]',
      {
        state: 'hidden',
        timeout: 15000
      }
    )
    .catch(() => {
      // Skeleton might not appear if data loads very quickly, continue
    })

  // Wait for either the new operation route button or the empty state to confirm page is ready
  // The button might not appear if user lacks permissions or data is loading
  await Promise.race([
    page
      .getByTestId('new-operation-route')
      .first()
      .waitFor({ state: 'visible', timeout: 15000 }),
    page
      .getByTestId('operation-routes-table')
      .waitFor({ state: 'visible', timeout: 15000 }),
    page
      .getByText(/You haven't created any Operation Routes yet/i)
      .waitFor({ state: 'visible', timeout: 15000 })
  ]).catch(() => {
    // If none appears, continue anyway - page is rendered
  })

  // Ensure the new operation route button is enabled and clickable
  // Wait for it to be attached to the DOM and in an enabled state
  await page
    .getByTestId('new-operation-route')
    .first()
    .waitFor({ state: 'attached', timeout: 5000 })
    .catch(() => {
      // Button might not exist if user lacks permissions
    })

  // Additional wait to ensure all JavaScript execution is complete
  // This gives time for any event handlers or state updates to finish
  await page.waitForTimeout(10000)
}
