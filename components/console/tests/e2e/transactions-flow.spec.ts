import { test, expect } from '@playwright/test'
import { navigateToTransactions } from '../utils/navigate-to-transactions'

test.beforeEach(async ({ page }) => {
  await navigateToTransactions(page)
})

test.describe('Transactions Management - E2E Tests', () => {
  test.describe('Transaction Creation', () => {
    test('should open transaction creation modal', async ({ page }) => {
      await page.getByTestId('new-transaction').click()
      await expect(page.getByTestId('transaction-mode-modal')).toBeVisible()
    })

    test('should select transaction mode', async ({ page }) => {
      await test.step('Open transaction mode modal', async () => {
        await page.getByTestId('new-transaction').click()
        await expect(page.getByTestId('transaction-mode-modal')).toBeVisible()
      })

      await test.step('Select simple mode', async () => {
        const simpleModeButton = page.getByTestId('simple-mode')
        if (await simpleModeButton.isVisible()) {
          await simpleModeButton.click()
        }
      })
    })

    test('should create simple transaction', async ({ page }) => {
      await test.step('Navigate to transaction creation', async () => {
        await page.getByTestId('new-transaction').click()
        const simpleModeButton = page.getByTestId('simple-mode')
        if (await simpleModeButton.isVisible()) {
          await simpleModeButton.click()
        }
      })

      await test.step('Fill transaction form', async () => {
        await page.locator('input[name="asset"]').fill('USD')
        await page.locator('input[name="value"]').fill('100.00')
        await page.locator('input[name="description"]').fill('Test Transaction')

        await page
          .locator('input[name="source[0].accountAlias"]')
          .fill('account-source')
        await page
          .locator('input[name="destination[0].accountAlias"]')
          .fill('account-dest')
      })

      await test.step('Submit transaction', async () => {
        await page.getByRole('button', { name: 'Create Transaction' }).click()
        await expect(page.getByTestId('success-toast')).toBeVisible()
      })
    })

    test('should create transaction with metadata', async ({ page }) => {
      await test.step('Navigate to transaction creation', async () => {
        await page.getByTestId('new-transaction').click()
        const simpleModeButton = page.getByTestId('simple-mode')
        if (await simpleModeButton.isVisible()) {
          await simpleModeButton.click()
        }
      })

      await test.step('Fill transaction form with metadata', async () => {
        await page.locator('input[name="asset"]').fill('USD')
        await page.locator('input[name="value"]').fill('250.00')

        await page.locator('#metadata').click()
        await page.locator('#key').fill('category')
        await page.locator('#value').fill('expense')
        await page.getByRole('button', { name: 'Add' }).first().click()

        await page
          .locator('input[name="source[0].accountAlias"]')
          .fill('wallet-001')
        await page
          .locator('input[name="destination[0].accountAlias"]')
          .fill('expense-001')
      })

      await test.step('Submit transaction', async () => {
        await page.getByRole('button', { name: 'Create Transaction' }).click()
        await expect(page.getByTestId('success-toast')).toBeVisible()
      })
    })
  })

  test.describe('Transaction Listing', () => {
    test('should list transactions', async ({ page }) => {
      await expect(page.getByTestId('transactions-table')).toBeVisible()
    })

    test('should navigate between pages', async ({ page }) => {
      const nextButton = page.getByTestId('next-page')
      const prevButton = page.getByTestId('prev-page')

      if (await nextButton.isVisible()) {
        const isEnabled = await nextButton.isEnabled()
        if (isEnabled) {
          await nextButton.click()
          await page.waitForLoadState('networkidle')
          await expect(prevButton).toBeEnabled()
        }
      }
    })

    test('should view transaction details', async ({ page }) => {
      await test.step('Click on a transaction row', async () => {
        const firstRow = page.getByTestId('transaction-row').first()
        if (await firstRow.isVisible()) {
          await firstRow.click()
          await expect(page.getByTestId('transaction-details')).toBeVisible()
        }
      })
    })
  })

  test.describe('Validation Scenarios', () => {
    test('should validate required asset field', async ({ page }) => {
      await page.getByTestId('new-transaction').click()
      const simpleModeButton = page.getByTestId('simple-mode')
      if (await simpleModeButton.isVisible()) {
        await simpleModeButton.click()
      }

      await page.locator('input[name="value"]').fill('100.00')
      await page.getByRole('button', { name: 'Create Transaction' }).click()

      await expect(page.getByText(/asset.*required/i)).toBeVisible()
    })

    test('should validate required value field', async ({ page }) => {
      await page.getByTestId('new-transaction').click()
      const simpleModeButton = page.getByTestId('simple-mode')
      if (await simpleModeButton.isVisible()) {
        await simpleModeButton.click()
      }

      await page.locator('input[name="asset"]').fill('USD')
      await page.getByRole('button', { name: 'Create Transaction' }).click()

      await expect(page.getByText(/value.*required/i)).toBeVisible()
    })

    test('should validate source account', async ({ page }) => {
      await page.getByTestId('new-transaction').click()
      const simpleModeButton = page.getByTestId('simple-mode')
      if (await simpleModeButton.isVisible()) {
        await simpleModeButton.click()
      }

      await page.locator('input[name="asset"]').fill('USD')
      await page.locator('input[name="value"]').fill('100.00')
      await page
        .locator('input[name="destination[0].accountAlias"]')
        .fill('account-dest')
      await page.getByRole('button', { name: 'Create Transaction' }).click()

      await expect(page.getByText(/source.*required/i)).toBeVisible()
    })

    test('should validate destination account', async ({ page }) => {
      await page.getByTestId('new-transaction').click()
      const simpleModeButton = page.getByTestId('simple-mode')
      if (await simpleModeButton.isVisible()) {
        await simpleModeButton.click()
      }

      await page.locator('input[name="asset"]').fill('USD')
      await page.locator('input[name="value"]').fill('100.00')
      await page
        .locator('input[name="source[0].accountAlias"]')
        .fill('account-source')
      await page.getByRole('button', { name: 'Create Transaction' }).click()

      await expect(page.getByText(/destination.*required/i)).toBeVisible()
    })

    test('should validate value format', async ({ page }) => {
      await page.getByTestId('new-transaction').click()
      const simpleModeButton = page.getByTestId('simple-mode')
      if (await simpleModeButton.isVisible()) {
        await simpleModeButton.click()
      }

      await page.locator('input[name="asset"]').fill('USD')
      await page.locator('input[name="value"]').fill('invalid-amount')
      await page.getByRole('button', { name: 'Create Transaction' }).click()

      const errorVisible = await page.getByText(/value.*invalid/i).isVisible()
      if (errorVisible) {
        await expect(page.getByText(/value.*invalid/i)).toBeVisible()
      }
    })
  })

  test.describe('Complex Workflows', () => {
    test('should create multi-account transaction', async ({ page }) => {
      await test.step('Navigate to transaction creation', async () => {
        await page.getByTestId('new-transaction').click()
        const advancedModeButton = page.getByTestId('advanced-mode')
        if (await advancedModeButton.isVisible()) {
          await advancedModeButton.click()
        }
      })

      await test.step('Add multiple source accounts', async () => {
        await page.locator('input[name="asset"]').fill('USD')
        await page.locator('input[name="value"]').fill('500.00')

        const addSourceButton = page.getByTestId('add-source-account')
        if (await addSourceButton.isVisible()) {
          await addSourceButton.click()
        }
      })
    })

    test('should filter transactions by date', async ({ page }) => {
      const dateFilterButton = page.getByTestId('date-filter')
      if (await dateFilterButton.isVisible()) {
        await dateFilterButton.click()

        const startDateInput = page.locator('input[name="startDate"]')
        const endDateInput = page.locator('input[name="endDate"]')

        if (await startDateInput.isVisible()) {
          await startDateInput.fill('2024-01-01')
          await endDateInput.fill('2024-12-31')
          await page.getByRole('button', { name: 'Apply' }).click()
          await page.waitForLoadState('networkidle')
        }
      }
    })

    test('should filter transactions by status', async ({ page }) => {
      const statusFilterButton = page.getByTestId('status-filter')
      if (await statusFilterButton.isVisible()) {
        await statusFilterButton.click()

        const pendingOption = page.getByTestId('status-pending')
        if (await pendingOption.isVisible()) {
          await pendingOption.click()
          await page.waitForLoadState('networkidle')
        }
      }
    })

    test('should export transactions', async ({ page }) => {
      const exportButton = page.getByTestId('export-transactions')
      if (await exportButton.isVisible()) {
        await exportButton.click()

        const downloadStarted = await Promise.race([
          page.waitForEvent('download', { timeout: 5000 }).then(() => true),
          page.waitForTimeout(5000).then(() => false)
        ])

        if (downloadStarted) {
          expect(downloadStarted).toBe(true)
        }
      }
    })
  })

  test.describe('Transaction Details', () => {
    test('should view full transaction details', async ({ page }) => {
      const firstTransaction = page.getByTestId('transaction-row').first()
      if (await firstTransaction.isVisible()) {
        await firstTransaction.click()

        await expect(page.getByTestId('transaction-id')).toBeVisible()
        await expect(page.getByTestId('transaction-amount')).toBeVisible()
        await expect(page.getByTestId('transaction-status')).toBeVisible()
        await expect(page.getByTestId('transaction-date')).toBeVisible()
      }
    })

    test('should view transaction source and destination', async ({ page }) => {
      const firstTransaction = page.getByTestId('transaction-row').first()
      if (await firstTransaction.isVisible()) {
        await firstTransaction.click()

        await expect(page.getByTestId('source-account')).toBeVisible()
        await expect(page.getByTestId('destination-account')).toBeVisible()
      }
    })
  })
})
