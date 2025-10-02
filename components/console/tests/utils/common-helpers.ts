import { Page, expect } from '@playwright/test'

/**
 * Common Test Helpers
 * Reusable utility functions for E2E tests
 */
export class CommonHelpers {
  /**
   * Fill a form with provided data
   * Automatically handles different input types (text, checkbox, select)
   */
  static async fillForm(
    page: Page,
    formData: Record<string, any>
  ): Promise<void> {
    for (const [field, value] of Object.entries(formData)) {
      const input = page.locator(`[name="${field}"]`)

      // Check if element exists
      if (!(await input.count())) {
        console.warn(`Field "${field}" not found in form`)
        continue
      }

      const inputType = await input.getAttribute('type')
      const tagName = await input.evaluate((el) => el.tagName.toLowerCase())

      if (inputType === 'checkbox' || inputType === 'radio') {
        if (value) {
          await input.check()
        } else {
          await input.uncheck()
        }
      } else if (tagName === 'select') {
        await input.selectOption(String(value))
      } else {
        await input.fill(String(value))
      }
    }
  }

  /**
   * Add metadata key-value pairs
   */
  static async addMetadata(
    page: Page,
    metadata: Record<string, string>
  ): Promise<void> {
    // Click to expand metadata section if needed
    const metadataToggle = page.locator('#metadata')
    if (await metadataToggle.isVisible()) {
      await metadataToggle.click()
    }

    for (const [key, value] of Object.entries(metadata)) {
      await page.locator('#key').fill(key)
      await page.locator('#value').fill(value)
      await page.getByRole('button', { name: 'Add' }).first().click()
    }
  }

  /**
   * Wait for toast message and dismiss it
   */
  static async waitForToast(
    page: Page,
    type: 'success' | 'error' = 'success',
    timeout: number = 5000
  ): Promise<void> {
    const toast = page.getByTestId(`${type}-toast`)
    await expect(toast).toBeVisible({ timeout })

    // Dismiss toast if dismiss button is available
    const dismissButton = page.getByTestId('dismiss-toast')
    if (await dismissButton.isVisible()) {
      await dismissButton.click()
      await expect(toast).not.toBeVisible()
    }
  }

  /**
   * Confirm dialog/modal action
   */
  static async confirmDialog(page: Page): Promise<void> {
    const confirmButton = page.getByTestId('confirm')
    await expect(confirmButton).toBeVisible()
    await confirmButton.click()

    // Wait for dialog to close
    const dialog = page.getByTestId('dialog')
    if (await dialog.isVisible()) {
      await expect(dialog).not.toBeVisible()
    }
  }

  /**
   * Cancel dialog/modal action
   */
  static async cancelDialog(page: Page): Promise<void> {
    const cancelButton = page.getByTestId('cancel')
    await expect(cancelButton).toBeVisible()
    await cancelButton.click()

    // Wait for dialog to close
    const dialog = page.getByTestId('dialog')
    if (await dialog.isVisible()) {
      await expect(dialog).not.toBeVisible()
    }
  }

  /**
   * Search for items using search input
   */
  static async searchFor(page: Page, query: string): Promise<void> {
    const searchInput = page.getByTestId('search-input')
    await expect(searchInput).toBeVisible()
    await searchInput.fill(query)
    await page.waitForLoadState('networkidle')
  }

  /**
   * Clear search input
   */
  static async clearSearch(page: Page): Promise<void> {
    const searchInput = page.getByTestId('search-input')
    if (await searchInput.isVisible()) {
      await searchInput.clear()
      await page.waitForLoadState('networkidle')
    }
  }

  /**
   * Open actions dropdown for a row
   */
  static async openRowActions(
    page: Page,
    rowIdentifier: string | RegExp
  ): Promise<void> {
    const row = page.getByRole('row', { name: rowIdentifier })
    await expect(row).toBeVisible()
    await row.getByTestId('actions').click()
  }

  /**
   * Click edit action in row dropdown
   */
  static async clickEditAction(
    page: Page,
    rowIdentifier: string | RegExp
  ): Promise<void> {
    await this.openRowActions(page, rowIdentifier)
    await page.getByTestId('edit').click()
  }

  /**
   * Click delete action in row dropdown
   */
  static async clickDeleteAction(
    page: Page,
    rowIdentifier: string | RegExp
  ): Promise<void> {
    await this.openRowActions(page, rowIdentifier)
    await page.getByTestId('delete').click()
  }

  /**
   * Delete entity with confirmation
   */
  static async deleteEntityWithConfirmation(
    page: Page,
    rowIdentifier: string | RegExp
  ): Promise<void> {
    await this.clickDeleteAction(page, rowIdentifier)
    await this.confirmDialog(page)
    await this.waitForToast(page, 'success')
  }

  /**
   * Wait for network to be idle
   */
  static async waitForNetworkIdle(page: Page): Promise<void> {
    await page.waitForLoadState('networkidle')
  }

  /**
   * Wait for specific element to be visible
   */
  static async waitForElement(
    page: Page,
    selector: string,
    timeout: number = 5000
  ): Promise<void> {
    await page.waitForSelector(selector, { state: 'visible', timeout })
  }

  /**
   * Check if element exists
   */
  static async elementExists(page: Page, selector: string): Promise<boolean> {
    return (await page.locator(selector).count()) > 0
  }

  /**
   * Get table row count
   */
  static async getTableRowCount(
    page: Page,
    tableTestId: string
  ): Promise<number> {
    const table = page.getByTestId(tableTestId)
    const rows = table.locator('tbody tr')
    return await rows.count()
  }

  /**
   * Verify row exists in table
   */
  static async verifyRowExists(
    page: Page,
    rowIdentifier: string | RegExp
  ): Promise<void> {
    const row = page.getByRole('row', { name: rowIdentifier })
    await expect(row).toBeVisible()
  }

  /**
   * Verify row does not exist in table
   */
  static async verifyRowNotExists(
    page: Page,
    rowIdentifier: string | RegExp
  ): Promise<void> {
    const row = page.getByRole('row', { name: rowIdentifier })
    await expect(row).not.toBeVisible()
  }

  /**
   * Open sheet/modal for creating new entity
   */
  static async openCreateSheet(page: Page, entityName: string): Promise<void> {
    await page.getByTestId(`new-${entityName}`).click()
    await expect(page.getByTestId(`${entityName}-sheet`)).toBeVisible()
  }

  /**
   * Close sheet/modal
   */
  static async closeSheet(page: Page, entityName: string): Promise<void> {
    const sheet = page.getByTestId(`${entityName}-sheet`)
    if (await sheet.isVisible()) {
      // Try clicking close button
      const closeButton = sheet.getByRole('button', { name: 'Close' })
      if (await closeButton.isVisible()) {
        await closeButton.click()
      }
      await expect(sheet).not.toBeVisible()
    }
  }

  /**
   * Submit form (click Save button)
   */
  static async submitForm(page: Page): Promise<void> {
    await page.getByRole('button', { name: 'Save' }).click()
  }

  /**
   * Verify validation error message
   */
  static async verifyValidationError(
    page: Page,
    errorPattern: string | RegExp
  ): Promise<void> {
    const error = page.getByText(errorPattern)
    await expect(error).toBeVisible()
  }

  /**
   * Verify no validation errors
   */
  static async verifyNoValidationErrors(page: Page): Promise<void> {
    // Common error message patterns
    const errorPatterns = [/required/i, /invalid/i, /error/i]

    for (const pattern of errorPatterns) {
      const errors = page.getByText(pattern)
      const count = await errors.count()
      if (count > 0) {
        throw new Error(`Found validation error matching ${pattern}`)
      }
    }
  }

  /**
   * Select option from dropdown
   */
  static async selectOption(
    page: Page,
    fieldName: string,
    optionValue: string
  ): Promise<void> {
    const select = page.locator(`[name="${fieldName}"]`)
    await select.selectOption(optionValue)
  }

  /**
   * Toggle switch/checkbox
   */
  static async toggleSwitch(
    page: Page,
    fieldName: string,
    checked: boolean = true
  ): Promise<void> {
    const switchElement = page.locator(`[name="${fieldName}"]`)
    if (checked) {
      await switchElement.check()
    } else {
      await switchElement.uncheck()
    }
  }

  /**
   * Take screenshot with custom name
   */
  static async takeScreenshot(page: Page, name: string): Promise<void> {
    await page.screenshot({ path: `test-results/${name}.png`, fullPage: true })
  }

  /**
   * Scroll element into view
   */
  static async scrollIntoView(page: Page, selector: string): Promise<void> {
    await page.locator(selector).scrollIntoViewIfNeeded()
  }

  /**
   * Wait for specific timeout (use sparingly)
   */
  static async wait(ms: number): Promise<void> {
    await new Promise((resolve) => setTimeout(resolve, ms))
  }

  /**
   * Get current URL
   */
  static async getCurrentUrl(page: Page): Promise<string> {
    return page.url()
  }

  /**
   * Verify URL contains path
   */
  static async verifyUrlContains(
    page: Page,
    pathPattern: string | RegExp
  ): Promise<void> {
    await expect(page).toHaveURL(pathPattern)
  }

  /**
   * Go back in browser history
   */
  static async goBack(page: Page): Promise<void> {
    await page.goBack()
    await page.waitForLoadState('networkidle')
  }

  /**
   * Reload page
   */
  static async reloadPage(page: Page): Promise<void> {
    await page.reload()
    await page.waitForLoadState('networkidle')
  }

  /**
   * Check if page contains text
   */
  static async pageContainsText(
    page: Page,
    text: string | RegExp
  ): Promise<boolean> {
    return await page.getByText(text).isVisible()
  }

  /**
   * Verify page title
   */
  static async verifyPageTitle(
    page: Page,
    title: string | RegExp
  ): Promise<void> {
    const titleElement = page.getByTestId('title')
    await expect(titleElement).toHaveText(title)
  }
}
