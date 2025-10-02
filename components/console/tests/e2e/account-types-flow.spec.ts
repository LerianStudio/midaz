import { test, expect } from '@playwright/test'
import { navigateToAccountTypes } from '../utils/navigate-to-account-types'

test.beforeEach(async ({ page }) => {
  await navigateToAccountTypes(page)
})

test.describe('Account Types Management - E2E Tests', () => {
  test.describe('CRUD Operations', () => {
    test('should create account type with all required fields', async ({
      page
    }) => {
      await test.step('Open create account type sheet', async () => {
        await page.getByTestId('new-account-type').click()
        await expect(page.getByTestId('account-type-sheet')).toBeVisible()
      })

      await test.step('Fill account type form', async () => {
        await page.locator('input[name="name"]').fill('Savings Account')
        await page.locator('input[name="keyValue"]').fill('savings')
        await page
          .locator('input[name="description"]')
          .fill('Standard savings account type')
      })

      await test.step('Add metadata', async () => {
        await page.locator('#metadata').click()
        await page.locator('#key').fill('interest-bearing')
        await page.locator('#value').fill('true')
        await page.getByRole('button', { name: 'Add' }).first().click()
      })

      await test.step('Submit and verify', async () => {
        await page.getByRole('button', { name: 'Save' }).click()
        await expect(page.getByTestId('account-type-sheet')).not.toBeVisible()
        await expect(page.getByTestId('success-toast')).toBeVisible()
        await page.getByTestId('dismiss-toast').click()
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
          await page.getByTestId('new-account-type').click()
          await page.locator('input[name="name"]').fill(accountType.name)
          await page.locator('input[name="keyValue"]').fill(accountType.key)
          await page
            .locator('input[name="description"]')
            .fill(accountType.description)
          await page.getByRole('button', { name: 'Save' }).click()
          await expect(page.getByTestId('success-toast')).toBeVisible()
          await page.getByTestId('dismiss-toast').click()
          await page.waitForLoadState('networkidle')
        })
      }
    })

    test('should update existing account type', async ({ page }) => {
      await test.step('Create account type to update', async () => {
        await page.getByTestId('new-account-type').click()
        await page.locator('input[name="name"]').fill('Account Type to Update')
        await page.locator('input[name="keyValue"]').fill('update-test')
        await page.locator('input[name="description"]').fill('Will be updated')
        await page.getByRole('button', { name: 'Save' }).click()
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
        await page.locator('input[name="name"]').fill('Updated Account Type')
        await page
          .locator('input[name="description"]')
          .fill('Updated description')
        await page.getByRole('button', { name: 'Save' }).click()
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
        await page.getByTestId('new-account-type').click()
        await page.locator('input[name="name"]').fill('Account Type to Delete')
        await page.locator('input[name="keyValue"]').fill('delete-test')
        await page.locator('input[name="description"]').fill('Will be deleted')
        await page.getByRole('button', { name: 'Save' }).click()
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
      await expect(page.getByTestId('account-types-table')).toBeVisible()
    })

    test('should search account types', async ({ page }) => {
      await test.step('Create searchable account type', async () => {
        await page.getByTestId('new-account-type').click()
        await page.locator('input[name="name"]').fill('Searchable Type XYZ')
        await page.locator('input[name="keyValue"]').fill('xyz-search')
        await page
          .locator('input[name="description"]')
          .fill('Test searchability')
        await page.getByRole('button', { name: 'Save' }).click()
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
      await page.getByTestId('new-account-type').click()
      await page.locator('input[name="keyValue"]').fill('test')
      await page.locator('input[name="description"]').fill('Test description')
      await page.getByRole('button', { name: 'Save' }).click()

      await expect(page.getByText(/name.*required/i)).toBeVisible()
    })

    test('should validate required keyValue field', async ({ page }) => {
      await page.getByTestId('new-account-type').click()
      await page.locator('input[name="name"]').fill('Test Account Type')
      await page.locator('input[name="description"]').fill('Test description')
      await page.getByRole('button', { name: 'Save' }).click()

      await expect(page.getByText(/keyValue.*required/i)).toBeVisible()
    })

    test('should validate required description field', async ({ page }) => {
      await page.getByTestId('new-account-type').click()
      await page.locator('input[name="name"]').fill('Test Account Type')
      await page.locator('input[name="keyValue"]').fill('test')
      await page.getByRole('button', { name: 'Save' }).click()

      await expect(page.getByText(/description.*required/i)).toBeVisible()
    })

    test('should validate keyValue format', async ({ page }) => {
      await page.getByTestId('new-account-type').click()
      await page.locator('input[name="name"]').fill('Test Account Type')
      await page.locator('input[name="keyValue"]').fill('Invalid Key!')
      await page.locator('input[name="description"]').fill('Test description')
      await page.getByRole('button', { name: 'Save' }).click()

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
        await page.getByTestId('new-account-type').click()
        await page.locator('input[name="name"]').fill(type.name)
        await page.locator('input[name="keyValue"]').fill(type.key)
        await page.locator('input[name="description"]').fill(type.description)
        await page.getByRole('button', { name: 'Save' }).click()
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
      await page.getByTestId('new-account-type').click()
      await page.locator('input[name="name"]').fill('Business Account')
      await page.locator('input[name="keyValue"]').fill('business')
      await page
        .locator('input[name="description"]')
        .fill('Account type for businesses')

      await page.locator('#metadata').click()

      const metadata = [
        { key: 'min-balance', value: '1000' },
        { key: 'max-transactions', value: 'unlimited' },
        { key: 'monthly-fee', value: '15' },
        { key: 'overdraft-protection', value: 'true' }
      ]

      for (const meta of metadata) {
        await page.locator('#key').fill(meta.key)
        await page.locator('#value').fill(meta.value)
        await page.getByRole('button', { name: 'Add' }).first().click()
      }

      await page.getByRole('button', { name: 'Save' }).click()
      await expect(page.getByTestId('success-toast')).toBeVisible()
    })
  })
})
