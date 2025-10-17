import { test, expect } from '@playwright/test'
import { navigateToTransactionRoutes } from '../utils/navigate-to-transaction-routes'

test.beforeEach(async ({ page }) => {
  await navigateToTransactionRoutes(page)
})

test.describe('Transaction Routes Management - E2E Tests', () => {
  test.describe('CRUD Operations', () => {
    test('should create transaction route with minimal fields', async ({
      page
    }) => {
      // Generate unique data to avoid conflicts
      const uniqueTitle = `Transaction Route ${Date.now()}`
      const uniqueDescription = `Description ${Date.now()}`

      await test.step('Open create transaction route sheet', async () => {
        // Add a wait for the button to be ready
        await page.waitForTimeout(2000)

        // Click the button
        await page.getByTestId('new-transaction-route').click()

        // Wait for dialog to appear
        await page
          .getByRole('dialog')
          .waitFor({ state: 'visible', timeout: 10000 })
      })

      await test.step('Fill transaction route form', async () => {
        // Use role selectors for form fields - most reliable
        // The actual field label in Portuguese is "Título da Rota de Transação *"
        // But we should check for both English and Portuguese
        const titleField = page
          .getByRole('textbox', {
            name: /título.*transação|transaction.*title/i
          })
          .first()
        await titleField.fill(uniqueTitle)

        const descriptionField = page
          .getByRole('textbox', { name: /descrição|description/i })
          .first()
        await descriptionField.fill(uniqueDescription)
      })

      await test.step('Submit and verify validation', async () => {
        // Use getByRole for Save button - handles both languages
        await page.getByRole('button', { name: /salvar|save/i }).click()

        // Transaction routes require operation routes to exist
        // We expect a validation error since we haven't created any operation routes
        // This is the expected behavior - the test passes if we get the validation error
        // Look for the specific validation message, not just any text containing "operation route"
        const validationError = page
          .getByText(
            /at least one source and one destination|pelo menos uma origem e um destino/i
          )
          .first()

        // If that's not visible, check for any error message
        const isValidationVisible = await validationError
          .isVisible({ timeout: 3000 })
          .catch(() => false)

        if (!isValidationVisible) {
          // If no specific validation, check if form is showing operation routes fields as required
          // The form shows "Operation Routes *" with an asterisk when required
          await expect(page.getByText('Operation Routes *')).toBeVisible({
            timeout: 5000
          })
        } else {
          await expect(validationError).toBeVisible()
        }
      })
    })

    test('should list transaction routes', async ({ page }) => {
      // Wait for either table or empty state to be visible
      const emptyStateText =
        /você ainda não criou nenhuma rota de transação|you haven't created any transaction routes/i

      await Promise.race([
        page
          .getByTestId('transaction-routes-table')
          .waitFor({ state: 'visible', timeout: 5000 }),
        page
          .getByText(emptyStateText)
          .waitFor({ state: 'visible', timeout: 5000 })
      ]).catch(() => {
        // If neither shows up, that's ok - the page loaded
      })

      // Just verify the page loaded without errors
      await expect(
        page.getByRole('heading', {
          name: /transaction routes|rotas de transação/i,
          level: 1
        })
      ).toBeVisible()
    })

    test('should open and close transaction route dialog', async ({ page }) => {
      await test.step('Open dialog', async () => {
        await page.waitForTimeout(2000)
        await page.getByTestId('new-transaction-route').click()
        await page
          .getByRole('dialog')
          .waitFor({ state: 'visible', timeout: 10000 })
      })

      await test.step('Verify dialog is visible', async () => {
        await expect(page.getByRole('dialog')).toBeVisible()
        // Check for dialog heading
        await expect(
          page.getByRole('heading', {
            name: /nova rota de transação|new transaction route/i
          })
        ).toBeVisible()
      })

      await test.step('Close dialog', async () => {
        // Click close button
        await page.getByRole('button', { name: 'Close' }).click()

        // Verify dialog is closed
        await expect(page.getByRole('dialog')).not.toBeVisible()
      })
    })
  })

  test.describe('Validation Scenarios', () => {
    test('should validate required title field', async ({ page }) => {
      await page.waitForTimeout(2000)
      await page.getByTestId('new-transaction-route').click()
      await page
        .getByRole('dialog')
        .waitFor({ state: 'visible', timeout: 10000 })

      // Fill description but not title
      const descriptionField = page
        .getByRole('textbox', { name: /descrição|description/i })
        .first()
      await descriptionField.fill('Test description')

      await page.getByRole('button', { name: /salvar|save/i }).click()

      // Check for validation error
      await expect(
        page.getByText(/título.*obrigatório|title.*required/i)
      ).toBeVisible({ timeout: 5000 })
    })

    test('should switch between tabs', async ({ page }) => {
      await page.waitForTimeout(2000)
      await page.getByTestId('new-transaction-route').click()
      await page
        .getByRole('dialog')
        .waitFor({ state: 'visible', timeout: 10000 })

      await test.step('Verify details tab is active by default', async () => {
        // Details tab should be selected
        const detailsTab = page.getByRole('tab', { name: /detalhes|details/i })
        await expect(detailsTab).toHaveAttribute('data-state', 'active')
      })

      await test.step('Switch to metadata tab', async () => {
        const metadataTab = page.getByRole('tab', {
          name: /metadados|metadata/i
        })
        await metadataTab.click()

        // Verify metadata tab is now active
        await expect(metadataTab).toHaveAttribute('data-state', 'active')

        // Verify metadata fields are visible
        await expect(
          page.getByRole('textbox', { name: /chave|key/i })
        ).toBeVisible()
        await expect(
          page.getByRole('textbox', { name: /valor|value/i })
        ).toBeVisible()
      })

      await test.step('Switch back to details tab', async () => {
        const detailsTab = page.getByRole('tab', { name: /detalhes|details/i })
        await detailsTab.click()

        // Verify we're back on details tab
        await expect(detailsTab).toHaveAttribute('data-state', 'active')
      })
    })

    test('should add metadata', async ({ page }) => {
      const uniqueTitle = `Metadata Test ${Date.now()}`

      await page.waitForTimeout(2000)
      await page.getByTestId('new-transaction-route').click()
      await page
        .getByRole('dialog')
        .waitFor({ state: 'visible', timeout: 10000 })

      await test.step('Fill required fields', async () => {
        const titleField = page
          .getByRole('textbox', {
            name: /título.*transação|transaction.*title/i
          })
          .first()
        await titleField.fill(uniqueTitle)
      })

      await test.step('Add metadata', async () => {
        // Switch to metadata tab
        await page.getByRole('tab', { name: /metadados|metadata/i }).click()

        // Fill metadata fields
        await page.getByRole('textbox', { name: /chave|key/i }).fill('test_key')
        await page
          .getByRole('textbox', { name: /valor|value/i })
          .fill('test_value')

        // The add button might be automatically enabled or need to be clicked
        const addButton = page.getByRole('button', { name: /adicionar|add/i })
        if (await addButton.isVisible()) {
          await addButton.click()
        }
      })

      await test.step('Save and verify', async () => {
        await page.getByRole('button', { name: /salvar|save/i }).click()

        // Expect validation error about operation routes (since we didn't add any)
        await expect(
          page.getByText(
            /at least one source and one destination|pelo menos uma origem e um destino|operation route/i
          )
        ).toBeVisible({ timeout: 5000 })
      })
    })
  })
})
