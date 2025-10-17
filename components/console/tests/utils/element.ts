import { Page } from '@playwright/test'

/**
 * Clicks an element identified by its data-testid attribute with comprehensive error handling and options.
 * This function provides reliable clicking with proper waiting, validation, and retry logic.
 *
 * @param page - Playwright page object
 * @param testId - The data-testid attribute value of the element to click
 * @param options - Configuration options for the click operation
 * @param options.waitForVisible - Whether to wait for the element to be visible (default: true)
 * @param options.waitForEnabled - Whether to wait for the element to be enabled (default: true)
 * @param options.timeout - Timeout in milliseconds for visibility/enabled checks (default: 5000)
 * @param options.force - Whether to force the click even if element is not actionable (default: false)
 * @param options.waitAfterClick - Time to wait after clicking in milliseconds (default: 100)
 * @returns Promise<boolean> - true if the click succeeded, false if it failed
 *
 * @example
 * // Basic usage - clicks with all default options
 * await click(page, 'submit-button')
 *
 * @example
 * // Click with custom timeout and forced click
 * await click(page, 'hidden-button', {
 *   timeout: 10000,
 *   force: true
 * })
 *
 * @example
 * // Fast click without waiting for enabled state
 * await click(page, 'quick-action', {
 *   waitForEnabled: false,
 *   waitAfterClick: 0
 * })
 */
export async function click(
  page: Page,
  testId: string,
  options: {
    waitForVisible?: boolean
    waitForEnabled?: boolean
    timeout?: number
    force?: boolean
    waitAfterClick?: number
  } = {}
): Promise<boolean> {
  try {
    const {
      waitForVisible = true,
      waitForEnabled = true,
      timeout = 5000,
      force = false,
      waitAfterClick = 100
    } = options

    const element = page.getByTestId(testId).first()

    // Check if element exists and is visible
    if (waitForVisible) {
      const elementExists = await element
        .isVisible({ timeout })
        .catch(() => false)
      if (!elementExists) {
        console.warn(`Element with testid "${testId}" not found or not visible`)
        return false
      }
    }

    // Wait for element to be enabled if requested
    if (waitForEnabled && !force) {
      const isEnabled = await element.isEnabled({ timeout }).catch(() => false)
      if (!isEnabled) {
        console.warn(`Element with testid "${testId}" is not enabled`)
        return false
      }
    }

    // Perform the click
    await element.click({ force, timeout })

    // Wait after click if requested
    if (waitAfterClick > 0) {
      await page.waitForTimeout(waitAfterClick)
    }

    return true
  } catch (error) {
    console.warn(`Error clicking element "${testId}":`, error)
    return false
  }
}
