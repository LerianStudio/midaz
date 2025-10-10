import { Page } from '@playwright/test'

/**
 * Types a value into an input field identified by its data-testid attribute.
 * This function provides comprehensive options for handling different input scenarios
 * with proper waiting, validation, and error handling.
 *
 * @param page - Playwright page object
 * @param testId - The data-testid attribute value of the input field
 * @param value - The text value to type into the input field
 * @param options - Configuration options for the input operation
 * @param options.clear - Whether to clear the field before typing (default: true)
 * @param options.waitForVisible - Whether to wait for the field to be visible (default: true)
 * @param options.waitForEnabled - Whether to wait for the field to be enabled (default: true)
 * @param options.timeout - Timeout in milliseconds for visibility/enabled checks (default: 5000)
 * @param options.validate - Whether to validate the entered value matches expected (default: true)
 * @returns Promise<boolean> - true if the operation succeeded, false if it failed
 *
 * @example
 * // Basic usage - types value with all default options
 * await inputType(page, 'email-input', 'user@example.com')
 *
 * @example
 * // Append to existing value without clearing
 * await inputType(page, 'search-input', 'additional text', { clear: false })
 *
 * @example
 * // Fast typing without validation for performance-critical tests
 * await inputType(page, 'bulk-input', 'data', {
 *   waitForEnabled: false,
 *   validate: false,
 *   timeout: 1000
 * })
 *
 * @example
 * // Type into a field that may take time to become available
 * await inputType(page, 'dynamic-input', 'value', {
 *   waitForVisible: true,
 *   waitForEnabled: true,
 *   timeout: 10000
 * })
 *
 * @example
 * // Handle special input types that don't need validation
 * await inputType(page, 'file-path-input', '/path/to/file', {
 *   validate: false // File inputs may transform the value
 * })
 */
export async function inputType(
  page: Page,
  testId: string,
  value: string,
  options: {
    clear?: boolean
    waitForVisible?: boolean
    waitForEnabled?: boolean
    timeout?: number
    validate?: boolean
  } = {}
): Promise<boolean> {
  try {
    const {
      clear = true,
      waitForVisible = true,
      waitForEnabled = true,
      timeout = 5000,
      validate = true
    } = options

    const inputField = page.getByTestId(testId).first()

    // Check if field exists and is visible
    if (waitForVisible) {
      const fieldExists = await inputField
        .isVisible({ timeout })
        .catch(() => false)
      if (!fieldExists) {
        console.warn(
          `Input field with testid "${testId}" not found or not visible`
        )
        return false
      }
    }

    // Wait for field to be enabled if requested
    if (waitForEnabled) {
      const isEnabled = await inputField
        .isEnabled({ timeout })
        .catch(() => false)
      if (!isEnabled) {
        console.warn(`Input field with testid "${testId}" is not enabled`)
        return false
      }
    }

    // Clear the field if requested
    if (clear) {
      await inputField.clear()
      await page.waitForTimeout(100)
    }

    // Fill the input field
    await inputField.fill(value)
    await page.waitForTimeout(200)

    // Validate the value was entered correctly if requested
    if (validate && value) {
      const actualValue = await inputField.inputValue().catch(() => '')
      if (actualValue !== value) {
        console.warn(
          `Input validation failed. Expected: "${value}", Actual: "${actualValue}"`
        )
        return false
      }
    }

    return true
  } catch (error) {
    console.warn(`Error typing in input field "${testId}":`, error)
    return false
  }
}

/**
 * Selects an option from a dropdown field by test ID
 * This function handles the common pattern of clicking a select field, waiting for options, and selecting one.
 *
 * @param page - Playwright page object
 * @param testId - The data-testid of the select field
 * @param value - Text content or index of the option to select ('first', 'last', number, or text content)
 * @param options - Configuration options for the select operation
 * @param options.waitForEnabled - Whether to wait for the field to be enabled before clicking (default: true)
 * @param options.timeout - Timeout for waiting operations in milliseconds (default: 5000)
 * @param options.optionsTimeout - Timeout for waiting for options to appear (default: 3000)
 * @param options.waitAfterSelection - Time to wait after selection in milliseconds (default: 1000)
 * @param options.fallbackToFirst - Whether to fallback to first option if text not found (default: true)
 * @returns Promise<boolean> - Whether the option was successfully selected
 *
 * @example
 * // Basic usage - select first option
 * await selectOption(page, 'database-select', 'first')
 *
 * @example
 * // Select by text content
 * await selectOption(page, 'status-select', 'Active')
 *
 * @example
 * // Select by index with custom options
 * await selectOption(page, 'priority-select', 2, {
 *   waitForEnabled: false,
 *   timeout: 10000,
 *   optionsTimeout: 5000
 * })
 *
 * @example
 * // Fast selection without fallback
 * await selectOption(page, 'category-select', 'Important', {
 *   fallbackToFirst: false,
 *   waitAfterSelection: 500
 * })
 */
export async function selectOption(
  page: Page,
  testId: string,
  value: string | number = 'first',
  options: {
    waitForEnabled?: boolean
    timeout?: number
    optionsTimeout?: number
    waitAfterSelection?: number
    fallbackToFirst?: boolean
  } = {}
): Promise<boolean> {
  try {
    const {
      waitForEnabled = true,
      timeout = 5000,
      optionsTimeout = 3000,
      waitAfterSelection = 1000,
      fallbackToFirst = true
    } = options

    const selectField = page.getByTestId(testId).first()

    // Check if field exists and is visible
    const fieldExists = await selectField.isVisible().catch(() => false)
    if (!fieldExists) {
      console.warn(
        `Select field with testid "${testId}" not found or not visible`
      )
      return false
    }

    // Wait for field to be enabled if requested
    if (waitForEnabled) {
      await page.waitForFunction(
        (testId) => {
          const element = document.querySelector(`[data-testid="${testId}"]`)
          return element && !element.hasAttribute('disabled')
        },
        testId,
        { timeout }
      )
    }

    // Click the select field to open options
    await selectField.click()

    // Wait for options to be populated
    await page.waitForFunction(
      () => {
        const options = document.querySelectorAll('[role="option"]')
        return options.length > 0
      },
      { timeout: optionsTimeout }
    )

    const optionsLocator = page.locator('[role="option"]')
    const optionCount = await optionsLocator.count()

    if (optionCount === 0) {
      await page.keyboard.press('Escape')
      return false
    }

    // Select the option based on the parameter
    let selectedOption = null

    if (value === 'first') {
      selectedOption = optionsLocator.first()
    } else if (value === 'last') {
      selectedOption = optionsLocator.last()
    } else if (typeof value === 'number') {
      if (value >= 0 && value < optionCount) {
        selectedOption = optionsLocator.nth(value)
      } else {
        console.warn(
          `Option index ${value} is out of range (0-${optionCount - 1})`
        )
        await page.keyboard.press('Escape')
        return false
      }
    } else if (typeof value === 'string') {
      // Search for option by text content
      selectedOption = optionsLocator.filter({ hasText: value }).first()
      const hasText = await selectedOption.isVisible().catch(() => false)
      if (!hasText) {
        if (fallbackToFirst) {
          // Fallback to first option if text not found
          console.warn(
            `Option with text "${value}" not found, selecting first option`
          )
          selectedOption = optionsLocator.first()
        } else {
          console.warn(
            `Option with text "${value}" not found, no fallback enabled`
          )
          await page.keyboard.press('Escape')
          return false
        }
      }
    }

    if (selectedOption) {
      await selectedOption.click()
      await page.waitForTimeout(waitAfterSelection) // Wait for selection to complete and potential API calls
      return true
    } else {
      await page.keyboard.press('Escape')
      return false
    }
  } catch (error) {
    console.warn(`Error selecting option in field "${testId}":`, error)
    await page.keyboard.press('Escape').catch(() => {})
    return false
  }
}
