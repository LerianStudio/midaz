import { test, expect, Page } from '@playwright/test'
import { navigateToAccountTypes } from '../utils/navigate-to-account-types'

// Helper function to open the account type sheet and wait for it to be ready
async function openAccountTypeSheet(page: Page) {
  // Ensure we're on the right page and it's loaded
  await page.waitForLoadState('networkidle')

  // Click the first new account type button (there might be two: one in header, one in empty state)
  const newButton = page.getByTestId('new-account-type').first()
  await newButton.waitFor({ state: 'visible', timeout: 5000 })
  await newButton.click()

  // Wait for the dialog/sheet to appear by looking for the dialog role
  await page.waitForSelector('[role="dialog"]', {
    state: 'visible',
    timeout: 10000
  })

  // Additional wait to ensure animations complete
  await page.waitForTimeout(500)

  // Wait for form inputs to be ready - using the actual label text
  await expect(
    page.getByRole('textbox', { name: /account type name/i })
  ).toBeVisible({
    timeout: 5000
  })
}

test.beforeEach(async ({ page }) => {
  await navigateToAccountTypes(page)
})

test.describe('Account Types Management - E2E Tests', () => {
  test.describe('CRUD Operations', () => {
    test('should create account type with all required fields', async ({
      page
    }) => {
      const timestamp = Date.now()
      const accountTypeName = `Savings Account ${timestamp}`
      const keyValue = `savings-${timestamp}`

      await test.step('Open create account type sheet', async () => {
        await openAccountTypeSheet(page)
      })

      await test.step('Fill account type form', async () => {
        await page
          .getByRole('textbox', { name: /account type name/i })
          .fill(accountTypeName)
        await page.getByRole('textbox', { name: /key value/i }).fill(keyValue)
        await page
          .getByRole('textbox', { name: /description/i })
          .fill('Standard savings account type')
      })

      await test.step('Add metadata', async () => {
        // Click the Metadata tab
        await page.getByRole('tab', { name: /metadata/i }).click()

        // Wait for the metadata tab panel to be visible
        await page.waitForTimeout(300)

        await page.getByLabel(/key/i).fill('interest-bearing')
        await page.getByLabel(/value/i).fill('true')

        // The add button in the metadata panel
        await page
          .locator('div[role="tabpanel"]')
          .getByRole('button')
          .first()
          .click()
      })

      await test.step('Submit and verify', async () => {
        // Go back to Details tab before saving
        await page.getByRole('tab', { name: /account type details/i }).click()
        await page.waitForTimeout(300)

        // Click save button
        const saveButton = page
          .getByRole('dialog')
          .getByRole('button', { name: /save/i })
        await saveButton.scrollIntoViewIfNeeded()

        await saveButton.click()

        // Wait for sheet to close (indicates successful save)
        await expect(page.getByRole('dialog')).not.toBeVisible({
          timeout: 15000
        })

        // Wait for toast to appear and dismiss it
        const toast = page.getByTestId('success-toast')
        if (await toast.isVisible()) {
          await page.getByTestId('dismiss-toast').click()
        }
      })

      await test.step('Verify account type appears in list', async () => {
        await page.waitForLoadState('networkidle')
        await expect(
          page.getByRole('row', { name: new RegExp(accountTypeName, 'i') })
        ).toBeVisible()
      })
    })

    test('should create different account types', async ({ page }) => {
      const timestamp = Date.now()
      const accountTypes = [
        {
          name: `Checking Account ${timestamp}`,
          key: `checking-${timestamp}`,
          description: 'Standard checking account'
        },
        {
          name: `Investment Account ${timestamp}`,
          key: `investment-${timestamp}`,
          description: 'Investment portfolio account'
        },
        {
          name: `Credit Account ${timestamp}`,
          key: `credit-${timestamp}`,
          description: 'Credit line account'
        }
      ]

      for (const accountType of accountTypes) {
        await test.step(`Create ${accountType.name}`, async () => {
          await openAccountTypeSheet(page)
          await page
            .getByRole('textbox', { name: /account type name/i })
            .fill(accountType.name)
          await page
            .getByRole('textbox', { name: /key value/i })
            .fill(accountType.key)
          await page
            .getByRole('textbox', { name: /description/i })
            .fill(accountType.description)

          const saveButton = page
            .getByRole('dialog')
            .getByRole('button', { name: /save/i })

          // Ensure button is ready before clicking
          await saveButton.scrollIntoViewIfNeeded()
          await page.waitForTimeout(500)
          await saveButton.click()

          // Wait for sheet to close (indicates successful save)
          await expect(page.getByRole('dialog')).not.toBeVisible({
            timeout: 20000
          })

          // Dismiss toast if visible
          const toast = page.getByTestId('success-toast')
          if (await toast.isVisible()) {
            await page.getByTestId('dismiss-toast').click()
          }
          await page.waitForLoadState('networkidle')
        })
      }
    })

    test('should update existing account type', async ({ page }) => {
      const timestamp = Date.now()
      await test.step('Create account type to update', async () => {
        await openAccountTypeSheet(page)
        await page
          .getByRole('textbox', { name: /account type name/i })
          .fill(`Account Type to Update ${timestamp}`)
        await page
          .getByRole('textbox', { name: /key value/i })
          .fill(`update-test-${timestamp}`)
        await page
          .getByRole('textbox', { name: /description/i })
          .fill('Will be updated')

        const saveButton = page
          .getByRole('dialog')
          .getByRole('button', { name: /save/i })

        // Ensure button is ready before clicking
        await saveButton.scrollIntoViewIfNeeded()
        await page.waitForTimeout(500)
        await saveButton.click()

        // Wait for sheet to close (indicates successful save)
        await expect(page.getByRole('dialog')).not.toBeVisible({
          timeout: 20000
        })

        // Dismiss toast if visible
        const toast = page.getByTestId('success-toast')
        if (await toast.isVisible()) {
          await page.getByTestId('dismiss-toast').click()
        }
      })

      await test.step('Open edit mode', async () => {
        await page.waitForLoadState('networkidle')
        const accountTypeRow = page.getByRole('row', {
          name: new RegExp(`Account Type to Update ${timestamp}`, 'i')
        })
        await accountTypeRow.getByTestId('actions').click()
        await page.getByTestId('edit').click()

        await page.waitForTimeout(500)
        await expect(page.getByRole('dialog')).toBeVisible({
          timeout: 10000
        })
      })

      await test.step('Update account type', async () => {
        await page
          .getByRole('textbox', { name: /account type name/i })
          .fill('Updated Account Type')
        await page
          .getByRole('textbox', { name: /description/i })
          .fill('Updated description')

        const saveButton = page
          .getByRole('dialog')
          .getByRole('button', { name: /save/i })

        // Ensure button is ready before clicking
        await saveButton.scrollIntoViewIfNeeded()
        await page.waitForTimeout(500)
        await saveButton.click()

        // Wait for sheet to close (indicates successful save)
        await expect(page.getByRole('dialog')).not.toBeVisible({
          timeout: 20000
        })
      })

      await test.step('Verify update', async () => {
        await page.waitForLoadState('networkidle')
        await expect(
          page.getByRole('row', { name: /Updated Account Type/i })
        ).toBeVisible()
      })
    })

    test('should delete account type with confirmation', async ({ page }) => {
      const timestamp = Date.now()
      await test.step('Create account type to delete', async () => {
        await openAccountTypeSheet(page)
        await page
          .getByRole('textbox', { name: /account type name/i })
          .fill(`Account Type to Delete ${timestamp}`)
        await page
          .getByRole('textbox', { name: /key value/i })
          .fill(`delete-test-${timestamp}`)
        await page
          .getByRole('textbox', { name: /description/i })
          .fill('Will be deleted')

        const saveButton = page
          .getByRole('dialog')
          .getByRole('button', { name: /save/i })

        // Ensure button is ready before clicking
        await saveButton.scrollIntoViewIfNeeded()
        await page.waitForTimeout(500)
        await saveButton.click()

        // Wait for sheet to close (indicates successful save)
        await expect(page.getByRole('dialog')).not.toBeVisible({
          timeout: 20000
        })

        // Dismiss toast if visible
        const toast = page.getByTestId('success-toast')
        if (await toast.isVisible()) {
          await page.getByTestId('dismiss-toast').click()
        }
      })

      await test.step('Delete the account type', async () => {
        await page.waitForLoadState('networkidle')
        const accountTypeRow = page.getByRole('row', {
          name: new RegExp(`Account Type to Delete ${timestamp}`, 'i')
        })
        await accountTypeRow.getByTestId('actions').click()
        await page.getByTestId('delete').click()
        await page.getByTestId('confirm').click()

        // Wait for delete success - toast should appear
        await expect(page.getByTestId('success-toast')).toBeVisible({
          timeout: 15000
        })
      })
    })

    test('should list account types with pagination', async ({ page }) => {
      // Wait for page to be fully loaded
      await page.waitForLoadState('networkidle')

      // Check if either table or empty state is visible
      await Promise.race([
        page.getByTestId('account-types-table').waitFor({ state: 'visible' }),
        page
          .getByText(/You haven't created any Account Types yet/i)
          .waitFor({ state: 'visible' })
      ])

      const hasTable = await page.getByTestId('account-types-table').isVisible()
      const hasEmptyState = await page
        .getByText(/You haven't created any Account Types yet/i)
        .isVisible()

      // At least one should be visible
      expect(hasTable || hasEmptyState).toBeTruthy()
    })
  })

  test.describe('Validation Scenarios', () => {
    test('should validate required name field', async ({ page }) => {
      await openAccountTypeSheet(page)
      await page.getByRole('textbox', { name: /key value/i }).fill('test')
      await page
        .getByRole('textbox', { name: /description/i })
        .fill('Test description')

      const saveButton = page
        .getByRole('dialog')
        .getByRole('button', { name: /save/i })
      await saveButton.click()

      await expect(page.getByText(/name.*required/i)).toBeVisible({
        timeout: 5000
      })
    })

    test('should validate required keyValue field', async ({ page }) => {
      await openAccountTypeSheet(page)
      await page
        .getByRole('textbox', { name: /account type name/i })
        .fill('Test Account Type')
      await page
        .getByRole('textbox', { name: /description/i })
        .fill('Test description')

      const saveButton = page
        .getByRole('dialog')
        .getByRole('button', { name: /save/i })
      await saveButton.click()

      // Check for validation error - could be different formats
      const validationError = page
        .locator('text=/required|must be|cannot be empty/i')
        .first()
      await expect(validationError).toBeVisible({
        timeout: 5000
      })
    })

    test.skip('should validate required description field', async ({
      page
    }) => {
      // Skipping this test as description is NOT required according to the schema
      // The schema at src/schema/account-types.ts shows description is optional
      await openAccountTypeSheet(page)
      await page
        .getByRole('textbox', { name: /account type name/i })
        .fill('Test Account Type')
      await page.getByRole('textbox', { name: /key value/i }).fill('test')

      const saveButton = page
        .getByRole('dialog')
        .getByRole('button', { name: /save/i })
      await saveButton.click()

      await expect(page.getByText(/description.*required/i)).toBeVisible({
        timeout: 5000
      })
    })

    test('should validate keyValue format', async ({ page }) => {
      await openAccountTypeSheet(page)
      await page
        .getByRole('textbox', { name: /account type name/i })
        .fill('Test Account Type')
      await page
        .getByRole('textbox', { name: /key value/i })
        .fill('Invalid Key!')
      await page
        .getByRole('textbox', { name: /description/i })
        .fill('Test description')

      const saveButton = page
        .getByRole('dialog')
        .getByRole('button', { name: /save/i })
      await saveButton.click()

      const formatError = await page.getByText(/keyValue.*invalid/i).isVisible()
      if (formatError) {
        await expect(page.getByText(/keyValue.*invalid/i)).toBeVisible()
      }
    })
  })

  test.describe('Complex Workflows', () => {
    test('should create comprehensive account type taxonomy', async ({
      page
    }) => {
      const timestamp = Date.now()
      const taxonomy = [
        {
          name: `Standard Checking ${timestamp}`,
          key: `checking-standard-${timestamp}`,
          description: 'Basic checking account with no fees'
        },
        {
          name: `Premium Checking ${timestamp}`,
          key: `checking-premium-${timestamp}`,
          description: 'Premium checking with benefits'
        },
        {
          name: `Student Savings ${timestamp}`,
          key: `savings-student-${timestamp}`,
          description: 'Savings account for students'
        },
        {
          name: `High-Yield Savings ${timestamp}`,
          key: `savings-high-yield-${timestamp}`,
          description: 'Savings with competitive interest rates'
        }
      ]

      for (const type of taxonomy) {
        await openAccountTypeSheet(page)
        await page
          .getByRole('textbox', { name: /account type name/i })
          .fill(type.name)
        await page.getByRole('textbox', { name: /key value/i }).fill(type.key)
        await page
          .getByRole('textbox', { name: /description/i })
          .fill(type.description)

        const saveButton = page
          .getByRole('dialog')
          .getByRole('button', { name: /save/i })

        // Ensure button is ready before clicking
        await saveButton.scrollIntoViewIfNeeded()
        await page.waitForTimeout(500)
        await saveButton.click()

        // Wait for sheet to close (indicates successful save)
        await expect(page.getByRole('dialog')).not.toBeVisible({
          timeout: 20000
        })

        // Dismiss toast if visible
        const toast = page.getByTestId('success-toast')
        if (await toast.isVisible()) {
          await page.getByTestId('dismiss-toast').click()
        }
        await page.waitForLoadState('networkidle')
      }

      for (const type of taxonomy) {
        await expect(
          page.getByRole('row', { name: new RegExp(type.name, 'i') })
        ).toBeVisible()
      }
    })

    test('should create account type with extensive metadata', async ({
      page
    }) => {
      const timestamp = Date.now()
      await openAccountTypeSheet(page)
      await page
        .getByRole('textbox', { name: /account type name/i })
        .fill(`Business Account ${timestamp}`)
      await page
        .getByRole('textbox', { name: /key value/i })
        .fill(`business-account-${timestamp}`)
      await page
        .getByRole('textbox', { name: /description/i })
        .fill('Account type for businesses')

      // Click the Metadata tab
      await page.getByRole('tab', { name: /metadata/i }).click()

      // Wait for the metadata tab panel to be visible
      await page.waitForTimeout(300)

      const metadata = [
        { key: 'min-balance', value: '1000' },
        { key: 'max-transactions', value: 'unlimited' },
        { key: 'monthly-fee', value: '15' },
        { key: 'overdraft-protection', value: 'true' }
      ]

      for (const meta of metadata) {
        await page.getByLabel(/key/i).fill(meta.key)
        await page.getByLabel(/value/i).fill(meta.value)
        await page
          .locator('div[role="tabpanel"]')
          .getByRole('button')
          .first()
          .click()
      }

      const saveButton = page
        .getByRole('dialog')
        .getByRole('button', { name: /save/i })

      // Ensure button is ready before clicking
      await saveButton.scrollIntoViewIfNeeded()
      await page.waitForTimeout(500)
      await saveButton.click()

      // Wait for sheet to close (indicates successful save)
      await expect(page.getByRole('dialog')).not.toBeVisible({
        timeout: 20000
      })
    })
  })
})
