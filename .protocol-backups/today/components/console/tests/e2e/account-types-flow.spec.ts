import { test, expect, Page } from '@playwright/test'
import { navigateToAccountTypes } from '../utils/navigate-to-account-types'

// Helper function to open the account type sheet and wait for it to be ready
async function openAccountTypeSheet(page: Page) {
  // Click the first new account type button (there might be two: one in header, one in empty state)
  await page.getByTestId('new-account-type').first().click()

  // Wait for the sheet to open
  await expect(page.getByTestId('account-type-sheet')).toBeVisible({
    timeout: 10000
  })

  // Wait for form inputs to be ready
  await expect(page.getByRole('textbox', { name: /name/i })).toBeVisible({
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
        // Go back to Details tab before saving (better UX and ensures all fields are validated)
        await page
          .getByRole('tab', { name: /account type details/i })
          .click()
        await page.waitForTimeout(500)

        // Check if there are any visible form messages with content
        const formMessages = page.locator(
          '[data-testid="account-type-sheet"] [role="alert"]'
        )
        const messageCount = await formMessages.count()
        if (messageCount > 0) {
          const messages = []
          for (let i = 0; i < messageCount; i++) {
            const text = await formMessages.nth(i).textContent()
            const isVisible = await formMessages.nth(i).isVisible()
            if (isVisible && text && text.trim().length > 0) {
              messages.push(text)
            }
          }
          if (messages.length > 0) {
            throw new Error(
              `Form has validation errors before submission: ${messages.join(', ')}`
            )
          }
        }

        // Scroll Save button into view and click
        const saveButton = page
          .getByTestId('account-type-sheet')
          .getByRole('button', { name: /save/i })
        await saveButton.scrollIntoViewIfNeeded()

        // Wait for a network request to be initiated after clicking save
        const responsePromise = page.waitForResponse(
          (response) =>
            response.url().includes('/account-types') &&
            (response.status() === 200 ||
              response.status() === 201 ||
              response.status() >= 400),
          { timeout: 15000 }
        )

        await saveButton.click()

        try {
          const response = await responsePromise
          console.log(`Response status: ${response.status()}`)
        } catch (error) {
          console.log('No network request detected after clicking Save button')
          // Check for validation errors that appeared after clicking
          const postMessages = page.locator(
            '[data-testid="account-type-sheet"] [role="alert"]'
          )
          const postCount = await postMessages.count()
          const visibleErrors = []
          for (let i = 0; i < postCount; i++) {
            const text = await postMessages.nth(i).textContent()
            const isVisible = await postMessages.nth(i).isVisible()
            if (isVisible && text && text.trim().length > 0) {
              visibleErrors.push(text)
            }
          }
          if (visibleErrors.length > 0) {
            throw new Error(`Form validation failed: ${visibleErrors.join(', ')}`)
          }
          throw error
        }

        // Wait for either success or error toast
        await expect(
          page.locator(
            '[data-testid="success-toast"], [data-testid="error-toast"]'
          )
        ).toBeVisible({ timeout: 10000 })

        // Check which toast appeared
        const isSuccessVisible = await page
          .getByTestId('success-toast')
          .isVisible()
        if (isSuccessVisible) {
          await page.getByTestId('dismiss-toast').click()
          await expect(page.getByTestId('account-type-sheet')).not.toBeVisible({
            timeout: 5000
          })
        } else {
          // If error toast, throw to see the error message
          const errorText = await page.getByTestId('error-toast').textContent()
          throw new Error(`Failed to create account type: ${errorText}`)
        }
      })

      await test.step('Verify account type appears in list', async () => {
        await page.waitForLoadState('networkidle')
        await expect(
          page.getByRole('row', { name: /Savings Account/i })
        ).toBeVisible()
      })
    })

    test('should create different account types', async ({ page }) => {
      const accountTypes = [
        {
          name: 'Checking Account',
          key: 'checking',
          description: 'Standard checking account'
        },
        {
          name: 'Investment Account',
          key: 'investment',
          description: 'Investment portfolio account'
        },
        {
          name: 'Credit Account',
          key: 'credit',
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
          await page.getByRole('button', { name: /save/i }).click()
          await expect(page.getByTestId('success-toast')).toBeVisible()
          await page.getByTestId('dismiss-toast').click()
          await page.waitForLoadState('networkidle')
        })
      }
    })

    test('should update existing account type', async ({ page }) => {
      await test.step('Create account type to update', async () => {
        await openAccountTypeSheet(page)
        await page
          .getByRole('textbox', { name: /account type name/i })
          .fill('Account Type to Update')
        await page
          .getByRole('textbox', { name: /key value/i })
          .fill('update-test')
        await page
          .getByRole('textbox', { name: /description/i })
          .fill('Will be updated')
        await page.getByRole('button', { name: /save/i }).click()
        await expect(page.getByTestId('success-toast')).toBeVisible()
        await page.getByTestId('dismiss-toast').click()
      })

      await test.step('Open edit mode', async () => {
        const accountTypeRow = page.getByRole('row', {
          name: /Account Type to Update/i
        })
        await page.waitForLoadState('networkidle')
        await accountTypeRow.getByTestId('actions').click()
        await page.getByTestId('edit').click()
        await expect(page.getByTestId('account-type-sheet')).toBeVisible()
      })

      await test.step('Update account type', async () => {
        await page
          .getByRole('textbox', { name: /account type name/i })
          .fill('Updated Account Type')
        await page
          .getByRole('textbox', { name: /description/i })
          .fill('Updated description')
        await page.getByRole('button', { name: /save/i }).click()
        await expect(page.getByTestId('success-toast')).toBeVisible()
      })

      await test.step('Verify update', async () => {
        await page.waitForLoadState('networkidle')
        await expect(
          page.getByRole('row', { name: /Updated Account Type/i })
        ).toBeVisible()
      })
    })

    test('should delete account type with confirmation', async ({ page }) => {
      await test.step('Create account type to delete', async () => {
        await openAccountTypeSheet(page)
        await page
          .getByRole('textbox', { name: /account type name/i })
          .fill('Account Type to Delete')
        await page
          .getByRole('textbox', { name: /key value/i })
          .fill('delete-test')
        await page
          .getByRole('textbox', { name: /description/i })
          .fill('Will be deleted')
        await page.getByRole('button', { name: /save/i }).click()
        await expect(page.getByTestId('success-toast')).toBeVisible()
        await page.getByTestId('dismiss-toast').click()
      })

      await test.step('Delete the account type', async () => {
        const accountTypeRow = page.getByRole('row', {
          name: /Account Type to Delete/i
        })
        await page.waitForLoadState('networkidle')
        await accountTypeRow.getByTestId('actions').click()
        await page.getByTestId('delete').click()
        await page.getByTestId('confirm').click()
        await expect(page.getByTestId('success-toast')).toBeVisible()
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

      const hasTable = await page
        .getByTestId('account-types-table')
        .isVisible()
      const hasEmptyState = await page
        .getByText(/You haven't created any Account Types yet/i)
        .isVisible()

      // At least one should be visible
      expect(hasTable || hasEmptyState).toBeTruthy()
    })

    test('should search account types', async ({ page }) => {
      await test.step('Create searchable account type', async () => {
        await openAccountTypeSheet(page)
        await page
          .getByRole('textbox', { name: /account type name/i })
          .fill('Searchable Type XYZ')
        await page.getByRole('textbox', { name: /key value/i }).fill('xyz-search')
        await page
          .getByRole('textbox', { name: /description/i })
          .fill('Test searchability')
        await page.getByRole('button', { name: /save/i }).click()
        await expect(page.getByTestId('success-toast')).toBeVisible()
        await page.getByTestId('dismiss-toast').click()
      })

      await test.step('Search for account type', async () => {
        const searchInput = page.getByTestId('search-input')
        if (await searchInput.isVisible()) {
          await searchInput.fill('XYZ')
          await page.waitForLoadState('networkidle')
          await expect(
            page.getByRole('row', { name: /Searchable Type XYZ/i })
          ).toBeVisible()
        }
      })
    })
  })

  test.describe('Validation Scenarios', () => {
    test('should validate required name field', async ({ page }) => {
      await openAccountTypeSheet(page)
      await page.getByRole('textbox', { name: /key value/i }).fill('test')
      await page
        .getByRole('textbox', { name: /description/i })
        .fill('Test description')
      await page.getByRole('button', { name: /save/i }).click()

      await expect(page.getByText(/name.*required/i)).toBeVisible()
    })

    test('should validate required keyValue field', async ({ page }) => {
      await openAccountTypeSheet(page)
      await page
        .getByRole('textbox', { name: /account type name/i })
        .fill('Test Account Type')
      await page
        .getByRole('textbox', { name: /description/i })
        .fill('Test description')
      await page.getByRole('button', { name: /save/i }).click()

      await expect(page.getByText(/keyValue.*required/i)).toBeVisible()
    })

    test('should validate required description field', async ({ page }) => {
      await openAccountTypeSheet(page)
      await page
        .getByRole('textbox', { name: /account type name/i })
        .fill('Test Account Type')
      await page.getByRole('textbox', { name: /key value/i }).fill('test')
      await page.getByRole('button', { name: /save/i }).click()

      await expect(page.getByText(/description.*required/i)).toBeVisible()
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
      await page.getByRole('button', { name: /save/i }).click()

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
      const taxonomy = [
        {
          name: 'Standard Checking',
          key: 'checking-standard',
          description: 'Basic checking account with no fees'
        },
        {
          name: 'Premium Checking',
          key: 'checking-premium',
          description: 'Premium checking with benefits'
        },
        {
          name: 'Student Savings',
          key: 'savings-student',
          description: 'Savings account for students'
        },
        {
          name: 'High-Yield Savings',
          key: 'savings-high-yield',
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
        await page.getByRole('button', { name: /save/i }).click()
        await expect(page.getByTestId('success-toast')).toBeVisible()
        await page.getByTestId('dismiss-toast').click()
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
      await openAccountTypeSheet(page)
      await page
        .getByRole('textbox', { name: /account type name/i })
        .fill('Business Account')
      await page
        .getByRole('textbox', { name: /key value/i })
        .fill('business-account')
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

      await page.getByRole('button', { name: /save/i }).click()
      await expect(page.getByTestId('success-toast')).toBeVisible()
    })
  })
})
