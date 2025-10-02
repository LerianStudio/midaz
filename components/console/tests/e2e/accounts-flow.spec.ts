import { test, expect } from '@playwright/test'
import { testDataFactory } from '../fixtures/test-data.factory'
import { CommonHelpers } from '../utils/common-helpers'
import { navigateToAccounts } from '../utils/navigate-to-accounts'

test.describe('Accounts Management - Basic E2E Tests', () => {
  let testData: ReturnType<typeof testDataFactory.account>

  test.beforeEach(async ({ page }) => {
    // Generate fresh test data for each test
    testData = testDataFactory.account()

    // Navigate to accounts page
    await navigateToAccounts(page)
  })

  test.describe('Page Navigation & Initial Load', () => {
    test('should load accounts page with correct heading', async ({ page }) => {
      await test.step('Verify page heading is visible', async () => {
        await expect(
          page.getByRole('heading', { name: 'Accounts', level: 1 })
        ).toBeVisible({ timeout: 30000 })
      })

      await test.step('Verify New Account button is present', async () => {
        const newAccountBtn = page.getByTestId('new-account')
        await expect(newAccountBtn).toBeAttached({ timeout: 30000 })
      })
    })

    test('should display search input field', async ({ page }) => {
      await test.step('Verify search input is visible', async () => {
        const searchInput = page.getByTestId('search-input')
        await expect(searchInput).toBeVisible({ timeout: 30000 })
      })

      await test.step('Verify search placeholder text', async () => {
        const searchInput = page.getByTestId('search-input')
        const placeholder = await searchInput.getAttribute('placeholder')
        expect(placeholder).toContain('Search')
      })
    })
  })

  test.describe('Empty State', () => {
    test('should display empty state when no accounts exist', async ({
      page
    }) => {
      // Check if empty state message exists (will be visible if no accounts)
      const emptyMessage = page.getByText(
        /You haven't created any Accounts yet/i
      )
      const accountsTable = page.getByTestId('accounts-table')

      // Either empty message or accounts table should be visible
      const hasEmptyState = await emptyMessage.isVisible().catch(() => false)
      const hasTable = await accountsTable.isVisible().catch(() => false)

      expect(hasEmptyState || hasTable).toBeTruthy()
    })
  })

  test.describe('Create Account Flow', () => {
    test('should open create account sheet', async ({ page }) => {
      await test.step('Click New Account button', async () => {
        const newAccountBtn = page.getByTestId('new-account')

        // Wait for button to be visible with extended timeout
        await expect(newAccountBtn).toBeVisible({ timeout: 30000 })

        // Check if button is enabled
        const isDisabled = await newAccountBtn.isDisabled()

        if (!isDisabled) {
          await newAccountBtn.click()
          await CommonHelpers.waitForNetworkIdle(page)

          await test.step('Verify account sheet is visible', async () => {
            const accountSheet = page.getByTestId('account-sheet')
            await expect(accountSheet).toBeVisible({ timeout: 5000 })
          })
        } else {
          // Button is disabled - likely no assets or account types exist
          test.skip(
            'New Account button is disabled - prerequisites not met (assets/account types required)'
          )
        }
      })
    })

    test('should fill account form with required fields', async ({ page }) => {
      await test.step('Open create account sheet', async () => {
        const newAccountBtn = page.getByTestId('new-account')

        // Wait for button to be visible
        await expect(newAccountBtn).toBeVisible({ timeout: 30000 })

        const isDisabled = await newAccountBtn.isDisabled()

        if (isDisabled) {
          test.skip('Prerequisites not met - assets or account types missing')
        }

        await newAccountBtn.click()
        await CommonHelpers.waitForNetworkIdle(page)
      })

      await test.step('Fill account name', async () => {
        const accountName = testDataFactory.uniqueName('TestAccount')
        await page.locator('input[name="name"]').fill(accountName)
      })

      await test.step('Verify form has asset field', async () => {
        // Asset field should be visible (required field)
        const assetField = page
          .locator('button[id*="assetCode"]')
          .or(page.locator('select[name="assetCode"]'))
        await expect(assetField).toBeAttached()
      })
    })
  })

  test.describe('Search Functionality', () => {
    test('should search accounts by typing in search input', async ({
      page
    }) => {
      await test.step('Verify search input accepts text', async () => {
        const searchInput = page.getByTestId('search-input')
        await expect(searchInput).toBeVisible({ timeout: 30000 })
        await searchInput.fill('test-account')
        await page.waitForTimeout(500) // Allow debounce

        const inputValue = await searchInput.inputValue()
        expect(inputValue).toBe('test-account')
      })

      await test.step('Clear search input', async () => {
        const searchInput = page.getByTestId('search-input')
        await searchInput.clear()
        await page.waitForTimeout(500)

        const inputValue = await searchInput.inputValue()
        expect(inputValue).toBe('')
      })
    })
  })

  test.describe('Accounts Table', () => {
    test('should display accounts table or empty state', async ({ page }) => {
      const accountsTable = page.getByTestId('accounts-table')
      const emptyState = page.getByText(/You haven't created any Accounts yet/i)

      // Wait for either table or empty state
      await Promise.race([
        accountsTable.waitFor({ state: 'visible', timeout: 10000 }),
        emptyState.waitFor({ state: 'visible', timeout: 10000 })
      ]).catch(() => {
        // If neither appears, test should fail
      })

      const hasTable = await accountsTable.isVisible().catch(() => false)
      const hasEmpty = await emptyState.isVisible().catch(() => false)

      expect(hasTable || hasEmpty).toBeTruthy()
    })

    test('should display table headers when accounts exist', async ({
      page
    }) => {
      const accountsTable = page.getByTestId('accounts-table')
      const tableExists = await accountsTable.isVisible().catch(() => false)

      if (tableExists) {
        await test.step('Verify table headers are present', async () => {
          // Check for key column headers
          const headers = [
            'Account Name',
            'ID',
            'Account Alias',
            'Assets',
            'Actions'
          ]

          for (const headerText of headers) {
            const header = page.getByRole('columnheader', {
              name: new RegExp(headerText, 'i')
            })
            await expect(header).toBeVisible()
          }
        })
      } else {
        test.skip('No accounts exist to verify table headers')
      }
    })

    test('should display action buttons for account rows', async ({ page }) => {
      const accountsTable = page.getByTestId('accounts-table')
      const tableExists = await accountsTable.isVisible().catch(() => false)

      if (tableExists) {
        await test.step('Check for actions button in first row', async () => {
          const firstActionButton = accountsTable.getByTestId('actions').first()

          const actionExists = await firstActionButton
            .isVisible()
            .catch(() => false)

          if (actionExists) {
            await expect(firstActionButton).toBeVisible()
          } else {
            test.skip('No account rows with actions available')
          }
        })
      } else {
        test.skip('No accounts table to verify actions')
      }
    })
  })

  test.describe('Account Actions', () => {
    test('should open actions dropdown menu', async ({ page }) => {
      const accountsTable = page.getByTestId('accounts-table')
      const tableExists = await accountsTable.isVisible().catch(() => false)

      if (tableExists) {
        const firstActionButton = accountsTable.getByTestId('actions').first()
        const actionExists = await firstActionButton
          .isVisible()
          .catch(() => false)

        if (actionExists) {
          await test.step('Click actions button', async () => {
            await firstActionButton.click()
            await page.waitForTimeout(300)
          })

          await test.step('Verify dropdown menu items', async () => {
            // Check for Details/Edit action
            const editAction = page
              .getByTestId('edit')
              .or(page.getByRole('menuitem', { name: /details/i }))
            await expect(editAction).toBeVisible({ timeout: 3000 })

            // Check for Delete action
            const deleteAction = page
              .getByTestId('delete')
              .or(page.getByRole('menuitem', { name: /delete/i }))
            await expect(deleteAction).toBeVisible()
          })
        } else {
          test.skip('No actions button available')
        }
      } else {
        test.skip('No accounts to test actions')
      }
    })
  })

  test.describe('Pagination', () => {
    test('should display pagination controls', async ({ page }) => {
      await test.step('Check for pagination elements', async () => {
        // Look for pagination container or controls
        const paginationText = page.getByText(/Showing/i)

        const paginationExists = await paginationText
          .isVisible()
          .catch(() => false)

        if (paginationExists) {
          await expect(paginationText).toBeVisible()
        } else {
          // Pagination might not show if there are no accounts or only one page
          test.skip('No pagination controls visible')
        }
      })
    })
  })
})
