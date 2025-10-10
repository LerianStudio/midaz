import type { Page } from '@playwright/test'

/**
 * Inputs metadata key-value pairs into the metadata fields
 * @param page - Playwright page object
 * @param metadata - Object containing key-value pairs to input
 */
export async function inputMetadata(
  page: Page,
  metadata: Record<string, string>
) {
  // Fill each metadata key-value pair
  for (const [key, value] of Object.entries(metadata)) {
    // Fill the key input
    const keyInput = page.getByTestId('metadata-key-input')
    await keyInput.waitFor({ state: 'visible', timeout: 5000 })
    await keyInput.fill(key)

    // Fill the value input
    const valueInput = page.getByTestId('metadata-value-input')
    await valueInput.fill(value)

    // Click the add button
    const addButton = page.getByTestId('metadata-add-button')
    await addButton.click()
    await page.waitForTimeout(300)
  }
}
