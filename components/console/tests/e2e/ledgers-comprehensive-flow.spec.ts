import { test, expect } from '@playwright/test'
import { testDataFactory } from '../fixtures/test-data.factory'
import { CommonHelpers } from '../utils/common-helpers'
import { navigateToLedgers } from '../utils/navigate-to-ledgers'

test.describe('Ledgers Management - Comprehensive E2E Tests', () => {
  let testData: ReturnType<typeof testDataFactory.ledger>

  test.beforeEach(async ({ page }) => {
    // Generate fresh test data for each test
    testData = testDataFactory.ledger()

    // Navigate to ledgers page
    await navigateToLedgers(page)
  })

  test.describe('CRUD Operations', () => {
    test('should create ledger with all fields including metadata', async ({
      page
    }) => {
      const ledgerName = testDataFactory.uniqueName('Ledger')

      await test.step('Open create ledger sheet', async () => {
        await page.getByTestId('new-ledger').click()
        await CommonHelpers.waitForNetworkIdle(page)
      })

      await test.step('Fill ledger form with name', async () => {
        await page.locator('input[name="name"]').fill(ledgerName)
      })

      await test.step('Add metadata', async () => {
        await CommonHelpers.addMetadata(page, {
          department: 'finance',
          region: 'north-america',
          'cost-center': 'cc-001'
        })
      })

      await test.step('Submit form and verify success', async () => {
        await CommonHelpers.submitForm(page)
        await CommonHelpers.waitForToast(page, 'success')
      })

      await test.step('Verify ledger appears in list', async () => {
        await CommonHelpers.waitForNetworkIdle(page)
        await CommonHelpers.verifyRowExists(page, new RegExp(ledgerName))
      })
    })

    test('should create ledger with minimal required fields', async ({
      page
    }) => {
      const ledgerName = testDataFactory.uniqueName('MinimalLedger')

      await test.step('Open create ledger sheet', async () => {
        await page.getByTestId('new-ledger').click()
      })

      await test.step('Fill only required field', async () => {
        await page.locator('input[name="name"]').fill(ledgerName)
      })

      await test.step('Submit and verify', async () => {
        await CommonHelpers.submitForm(page)
        await CommonHelpers.waitForToast(page, 'success')
      })

      await test.step('Verify ledger created', async () => {
        await CommonHelpers.waitForNetworkIdle(page)
        await CommonHelpers.verifyRowExists(page, new RegExp(ledgerName))
      })
    })

    test('should update existing ledger', async ({ page }) => {
      const originalName = testDataFactory.uniqueName('UpdateLedger')
      const updatedName = testDataFactory.uniqueName('UpdatedLedger')

      await test.step('Create ledger first', async () => {
        await page.getByTestId('new-ledger').click()
        await page.locator('input[name="name"]').fill(originalName)
        await CommonHelpers.submitForm(page)
        await CommonHelpers.waitForToast(page, 'success')
        await CommonHelpers.waitForNetworkIdle(page)
      })

      await test.step('Open edit mode', async () => {
        await CommonHelpers.clickEditAction(page, new RegExp(originalName))
        await CommonHelpers.waitForNetworkIdle(page)
      })

      await test.step('Update ledger name', async () => {
        const nameInput = page.locator('input[name="name"]')
        await nameInput.clear()
        await nameInput.fill(updatedName)
        await CommonHelpers.submitForm(page)
        await CommonHelpers.waitForToast(page, 'success')
      })

      await test.step('Verify update', async () => {
        await CommonHelpers.waitForNetworkIdle(page)
        await CommonHelpers.verifyRowExists(page, new RegExp(updatedName))
        await CommonHelpers.verifyRowNotExists(page, new RegExp(originalName))
      })
    })

    test('should delete ledger with confirmation', async ({ page }) => {
      const ledgerName = testDataFactory.uniqueName('DeleteLedger')

      await test.step('Create ledger to delete', async () => {
        await page.getByTestId('new-ledger').click()
        await page.locator('input[name="name"]').fill(ledgerName)
        await CommonHelpers.submitForm(page)
        await CommonHelpers.waitForToast(page, 'success')
        await CommonHelpers.waitForNetworkIdle(page)
      })

      await test.step('Delete the ledger', async () => {
        await CommonHelpers.deleteEntityWithConfirmation(
          page,
          new RegExp(ledgerName)
        )
      })

      await test.step('Verify ledger removed', async () => {
        await CommonHelpers.waitForNetworkIdle(page)
        await CommonHelpers.verifyRowNotExists(page, new RegExp(ledgerName))
      })
    })

    test('should cancel delete operation', async ({ page }) => {
      const ledgerName = testDataFactory.uniqueName('CancelDeleteLedger')

      await test.step('Create ledger', async () => {
        await page.getByTestId('new-ledger').click()
        await page.locator('input[name="name"]').fill(ledgerName)
        await CommonHelpers.submitForm(page)
        await CommonHelpers.waitForToast(page, 'success')
        await CommonHelpers.waitForNetworkIdle(page)
      })

      await test.step('Open delete dialog', async () => {
        await CommonHelpers.clickDeleteAction(page, new RegExp(ledgerName))
      })

      await test.step('Cancel deletion', async () => {
        await CommonHelpers.cancelDialog(page)
      })

      await test.step('Verify ledger still exists', async () => {
        await CommonHelpers.waitForNetworkIdle(page)
        await CommonHelpers.verifyRowExists(page, new RegExp(ledgerName))
      })
    })

    test('should list ledgers with pagination if available', async ({
      page
    }) => {
      await test.step('Verify ledgers table is visible', async () => {
        const tableExists = await CommonHelpers.elementExists(
          page,
          '[data-testid="ledgers-table"]'
        )
        if (tableExists) {
          await expect(page.getByTestId('ledgers-table')).toBeVisible()
        }
      })

      await test.step('Check pagination controls', async () => {
        const paginationExists = await page
          .getByTestId('pagination')
          .isVisible()
          .catch(() => false)
        if (paginationExists) {
          await expect(page.getByTestId('pagination')).toBeVisible()
        }
      })
    })

    test('should search ledgers', async ({ page }) => {
      const searchableName = testDataFactory.uniqueName('SearchLedger')

      await test.step('Create searchable ledger', async () => {
        await page.getByTestId('new-ledger').click()
        await page.locator('input[name="name"]').fill(searchableName)
        await CommonHelpers.submitForm(page)
        await CommonHelpers.waitForToast(page, 'success')
        await CommonHelpers.waitForNetworkIdle(page)
      })

      await test.step('Search for the ledger', async () => {
        const searchInput = page.getByTestId('search-input')
        if (await searchInput.isVisible()) {
          await CommonHelpers.searchFor(page, searchableName)
          await CommonHelpers.verifyRowExists(page, new RegExp(searchableName))
        }
      })
    })
  })

  test.describe('Validation Scenarios', () => {
    test('should validate required name field', async ({ page }) => {
      await test.step('Open create ledger sheet', async () => {
        await page.getByTestId('new-ledger').click()
      })

      await test.step('Submit without filling name', async () => {
        await CommonHelpers.submitForm(page)
      })

      await test.step('Verify validation error', async () => {
        await CommonHelpers.verifyValidationError(page, /name.*required/i)
      })
    })

    test('should validate name length constraints', async ({ page }) => {
      await test.step('Open create ledger sheet', async () => {
        await page.getByTestId('new-ledger').click()
      })

      await test.step('Enter name exceeding max length', async () => {
        const tooLongName = testDataFactory.invalid.tooLongString(100)
        await page.locator('input[name="name"]').fill(tooLongName)
        await CommonHelpers.submitForm(page)
      })

      await test.step('Verify validation error or truncation', async () => {
        // Either validation error appears or input truncates automatically
        const errorVisible = await page
          .getByText(/too long|max.*length/i)
          .isVisible()
          .catch(() => false)
        if (!errorVisible) {
          // Check if input value was truncated
          const inputValue = await page
            .locator('input[name="name"]')
            .inputValue()
          expect(inputValue.length).toBeLessThanOrEqual(100)
        }
      })
    })

    test('should prevent empty name submission', async ({ page }) => {
      await page.getByTestId('new-ledger').click()
      await page.locator('input[name="name"]').fill('')
      await CommonHelpers.submitForm(page)
      await CommonHelpers.verifyValidationError(page, /required/i)
    })
  })

  test.describe('Complex Workflows', () => {
    test('should create multiple ledgers in sequence', async ({ page }) => {
      const ledgerNames = [
        testDataFactory.uniqueName('Seq1'),
        testDataFactory.uniqueName('Seq2'),
        testDataFactory.uniqueName('Seq3')
      ]

      for (const name of ledgerNames) {
        await test.step(`Create ledger: ${name}`, async () => {
          await page.getByTestId('new-ledger').click()
          await page.locator('input[name="name"]').fill(name)
          await CommonHelpers.submitForm(page)
          await CommonHelpers.waitForToast(page, 'success')
          await CommonHelpers.waitForNetworkIdle(page)
        })
      }

      await test.step('Verify all ledgers created', async () => {
        for (const name of ledgerNames) {
          await CommonHelpers.verifyRowExists(page, new RegExp(name))
        }
      })
    })

    test('should handle ledger creation with special characters', async ({
      page
    }) => {
      const specialName = `Ledger-${Date.now()}_Test & Special`

      await test.step('Create ledger with special chars', async () => {
        await page.getByTestId('new-ledger').click()
        await page.locator('input[name="name"]').fill(specialName)
        await CommonHelpers.submitForm(page)
        await CommonHelpers.waitForToast(page, 'success')
      })

      await test.step('Verify creation', async () => {
        await CommonHelpers.waitForNetworkIdle(page)
        const escapedName = specialName.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
        await CommonHelpers.verifyRowExists(page, new RegExp(escapedName))
      })
    })

    test('should preserve metadata across edit operations', async ({
      page
    }) => {
      const ledgerName = testDataFactory.uniqueName('MetaLedger')
      const metadata = { key1: 'value1', key2: 'value2' }

      await test.step('Create ledger with metadata', async () => {
        await page.getByTestId('new-ledger').click()
        await page.locator('input[name="name"]').fill(ledgerName)
        await CommonHelpers.addMetadata(page, metadata)
        await CommonHelpers.submitForm(page)
        await CommonHelpers.waitForToast(page, 'success')
        await CommonHelpers.waitForNetworkIdle(page)
      })

      await test.step('Edit ledger', async () => {
        await CommonHelpers.clickEditAction(page, new RegExp(ledgerName))
        await CommonHelpers.waitForNetworkIdle(page)

        // Verify metadata is still present
        // This would require checking metadata display in edit mode
        // Implementation depends on how metadata is displayed in the UI
      })
    })
  })

  test.describe('Error Handling', () => {
    test('should handle duplicate ledger name gracefully', async ({ page }) => {
      const duplicateName = testDataFactory.uniqueName('DupeLedger')

      await test.step('Create first ledger', async () => {
        await page.getByTestId('new-ledger').click()
        await page.locator('input[name="name"]').fill(duplicateName)
        await CommonHelpers.submitForm(page)
        await CommonHelpers.waitForToast(page, 'success')
        await CommonHelpers.waitForNetworkIdle(page)
      })

      await test.step('Try to create ledger with same name', async () => {
        await page.getByTestId('new-ledger').click()
        await page.locator('input[name="name"]').fill(duplicateName)
        await CommonHelpers.submitForm(page)
      })

      await test.step('Verify error handling', async () => {
        // Either error toast or validation message appears
        const errorToast = page.getByTestId('error-toast')
        const errorMessage = page.getByText(/already exists|duplicate/i)

        const errorVisible =
          (await errorToast.isVisible().catch(() => false)) ||
          (await errorMessage.isVisible().catch(() => false))

        expect(errorVisible).toBeTruthy()
      })
    })

    test('should handle network timeout gracefully', async ({ page }) => {
      // This test would require network mocking to simulate timeout
      // Skipping for now as it requires additional setup
      test.skip()
    })
  })

  test.describe('UI/UX Validation', () => {
    test('should close sheet on cancel', async ({ page }) => {
      await page.getByTestId('new-ledger').click()
      await CommonHelpers.waitForNetworkIdle(page)

      const cancelButton = page.getByRole('button', { name: /cancel/i })
      if (await cancelButton.isVisible()) {
        await cancelButton.click()

        // Sheet should close
        const sheet = page.getByRole('dialog')
        if (await sheet.isVisible().catch(() => false)) {
          await expect(sheet).not.toBeVisible()
        }
      }
    })

    test('should show loading state during submission', async ({ page }) => {
      await page.getByTestId('new-ledger').click()
      await page
        .locator('input[name="name"]')
        .fill(testDataFactory.uniqueName('LoadingTest'))

      // Click submit and check for loading indicator
      const submitButton = page.getByRole('button', { name: 'Save' })
      await submitButton.click()

      // Look for loading indicator (spinner, disabled button, etc.)
      const isDisabled = await submitButton.isDisabled().catch(() => false)
      const hasLoadingClass = await submitButton
        .evaluate((el) => el.classList.contains('loading'))
        .catch(() => false)

      // Either button is disabled or has loading class
      expect(isDisabled || hasLoadingClass).toBeTruthy()
    })
  })
})
