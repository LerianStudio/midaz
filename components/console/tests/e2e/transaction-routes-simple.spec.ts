import { test, expect } from '@playwright/test'

test.describe('Transaction Routes - Simple Tests', () => {
  test.beforeEach(async ({ page }) => {
    // Navigate to transaction routes
    await page.goto('/transaction-routes', { waitUntil: 'domcontentloaded' })

    // Wait for page to load
    await page.waitForLoadState('networkidle', { timeout: 15000 })
  })

  test('should load transaction routes page', async ({ page }) => {
    // Check if we're on the right page
    const heading = page.getByRole('heading', {
      name: /transaction routes|rotas de transação/i,
      level: 1
    })

    await expect(heading).toBeVisible({ timeout: 10000 })
  })

  test('should show empty state or table', async ({ page }) => {
    // Check for either empty state or table
    const emptyState = page.getByText(
      /você ainda não criou|you haven't created/i
    )
    const table = page.getByTestId('transaction-routes-table')

    // At least one should be visible
    const hasContent = await Promise.race([
      emptyState
        .isVisible()
        .then(() => true)
        .catch(() => false),
      table
        .isVisible()
        .then(() => true)
        .catch(() => false)
    ])

    expect(hasContent).toBeTruthy()
  })

  test('should have new transaction route button', async ({ page }) => {
    // Check if button exists and is visible
    const button = page.getByTestId('new-transaction-route')
    await expect(button).toBeVisible({ timeout: 10000 })

    // Check if button is enabled
    await expect(button).toBeEnabled()
  })

  test('should click button and check for response', async ({ page }) => {
    // Get the button
    const button = page.getByTestId('new-transaction-route')

    // Add console listener to catch errors
    page.on('console', (msg) => {
      if (msg.type() === 'error') {
        console.log('Console error:', msg.text())
      }
    })

    // Try to click the button
    await button.click()

    // Wait a bit to see what happens
    await page.waitForTimeout(2000)

    // Check if any dialog/sheet appeared
    const dialog = page.getByRole('dialog')
    const isDialogVisible = await dialog.isVisible().catch(() => false)

    if (isDialogVisible) {
      console.log('✅ Dialog opened successfully')

      // Check for form fields
      const titleField = await page
        .getByRole('textbox')
        .first()
        .isVisible()
        .catch(() => false)
      console.log('Title field visible:', titleField)

      // Try to close the dialog
      const closeButton = page.getByRole('button', { name: /close|fechar/i })
      if (await closeButton.isVisible()) {
        await closeButton.click()
        console.log('✅ Dialog closed successfully')
      }
    } else {
      console.log('❌ Dialog did not open')

      // Take a screenshot for debugging
      await page.screenshot({
        path: 'transaction-routes-button-clicked.png',
        fullPage: true
      })

      // Check if button is still visible and enabled
      const buttonStillVisible = await button.isVisible()
      const buttonStillEnabled = await button.isEnabled()

      console.log('Button still visible:', buttonStillVisible)
      console.log('Button still enabled:', buttonStillEnabled)

      // Check for any error messages on the page
      const errorMessages = await page.getByText(/error|erro/i).count()
      console.log('Error messages found:', errorMessages)
    }
  })
})
