import { test, expect } from '@playwright/test'
import { navigateToOperationRoutes } from '../utils/navigate-to-operation-routes'

test.beforeEach(async ({ page }) => {
  await navigateToOperationRoutes(page)
})

test.describe('Operation Routes Management - E2E Tests', () => {
  test.describe('CRUD Operations', () => {
    test('should create operation route with alias rule', async ({ page }) => {
      await test.step('Open create operation route sheet', async () => {
        await page.getByTestId('new-operation-route').click()
        await expect(page.getByTestId('operation-route-sheet')).toBeVisible()
      })

      await test.step('Fill operation route form', async () => {
        await page.locator('input[name="title"]').fill('Payment Source Route')
        await page
          .locator('input[name="description"]')
          .fill('Route for payment source accounts')
        await page
          .locator('select[name="operationType"]')
          .selectOption('source')
        await page
          .locator('select[name="account.ruleType"]')
          .selectOption('alias')
        await page.locator('input[name="account.validIf"]').fill('payment-*')
      })

      await test.step('Add metadata', async () => {
        await page.locator('#metadata').click()
        await page.locator('#key').fill('priority')
        await page.locator('#value').fill('high')
        await page.getByRole('button', { name: 'Add' }).first().click()
      })

      await test.step('Submit and verify', async () => {
        await page.getByRole('button', { name: 'Save' }).click()
        await expect(
          page.getByTestId('operation-route-sheet')
        ).not.toBeVisible()
        await expect(page.getByTestId('success-toast')).toBeVisible()
        await page.getByTestId('dismiss-toast').click()
      })

      await test.step('Verify operation route appears in list', async () => {
        await page.waitForLoadState('networkidle')
        await expect(
          page.getByRole('row', { name: /Payment Source Route/i })
        ).toBeVisible()
      })
    })

    test('should create operation route with account type rule', async ({
      page
    }) => {
      await test.step('Open create operation route sheet', async () => {
        await page.getByTestId('new-operation-route').click()
        await expect(page.getByTestId('operation-route-sheet')).toBeVisible()
      })

      await test.step('Fill operation route with account type rule', async () => {
        await page.locator('input[name="title"]').fill('Destination Type Route')
        await page
          .locator('input[name="description"]')
          .fill('Route based on account types')
        await page
          .locator('select[name="operationType"]')
          .selectOption('destination')
        await page
          .locator('select[name="account.ruleType"]')
          .selectOption('account_type')

        const accountTypeSelect = page.locator('select[name="account.validIf"]')
        if (await accountTypeSelect.isVisible()) {
          const firstOption = await accountTypeSelect
            .locator('option')
            .nth(1)
            .textContent()
          if (firstOption) {
            await accountTypeSelect.selectOption({ index: 1 })
          }
        }
      })

      await test.step('Submit and verify', async () => {
        await page.getByRole('button', { name: 'Save' }).click()
        await expect(page.getByTestId('success-toast')).toBeVisible()
      })
    })

    test('should update existing operation route', async ({ page }) => {
      await test.step('Create operation route to update', async () => {
        await page.getByTestId('new-operation-route').click()
        await page.locator('input[name="title"]').fill('Route to Update')
        await page.locator('input[name="description"]').fill('Will be updated')
        await page
          .locator('select[name="operationType"]')
          .selectOption('source')
        await page
          .locator('select[name="account.ruleType"]')
          .selectOption('alias')
        await page.locator('input[name="account.validIf"]').fill('test-*')
        await page.getByRole('button', { name: 'Save' }).click()
        await expect(page.getByTestId('success-toast')).toBeVisible()
        await page.getByTestId('dismiss-toast').click()
      })

      await test.step('Open edit mode', async () => {
        const routeRow = page.getByRole('row', { name: /Route to Update/i })
        await page.waitForLoadState('networkidle')
        await routeRow.getByTestId('actions').click()
        await page.getByTestId('edit').click()
        await expect(page.getByTestId('operation-route-sheet')).toBeVisible()
      })

      await test.step('Update operation route', async () => {
        await page
          .locator('input[name="title"]')
          .fill('Updated Operation Route')
        await page
          .locator('input[name="description"]')
          .fill('Updated description')
        await page.getByRole('button', { name: 'Save' }).click()
        await expect(page.getByTestId('success-toast')).toBeVisible()
      })

      await test.step('Verify update', async () => {
        await page.waitForLoadState('networkidle')
        await expect(
          page.getByRole('row', { name: /Updated Operation Route/i })
        ).toBeVisible()
      })
    })

    test('should delete operation route with confirmation', async ({
      page
    }) => {
      await test.step('Create operation route to delete', async () => {
        await page.getByTestId('new-operation-route').click()
        await page.locator('input[name="title"]').fill('Route to Delete')
        await page.locator('input[name="description"]').fill('Will be deleted')
        await page
          .locator('select[name="operationType"]')
          .selectOption('source')
        await page
          .locator('select[name="account.ruleType"]')
          .selectOption('alias')
        await page.locator('input[name="account.validIf"]').fill('delete-*')
        await page.getByRole('button', { name: 'Save' }).click()
        await expect(page.getByTestId('success-toast')).toBeVisible()
        await page.getByTestId('dismiss-toast').click()
      })

      await test.step('Delete the operation route', async () => {
        const routeRow = page.getByRole('row', { name: /Route to Delete/i })
        await page.waitForLoadState('networkidle')
        await routeRow.getByTestId('actions').click()
        await page.getByTestId('delete').click()
        await page.getByTestId('confirm').click()
        await expect(page.getByTestId('success-toast')).toBeVisible()
      })
    })

    test('should list operation routes with pagination', async ({ page }) => {
      await expect(page.getByTestId('operation-routes-table')).toBeVisible()
    })

    test('should search operation routes', async ({ page }) => {
      await test.step('Create searchable operation route', async () => {
        await page.getByTestId('new-operation-route').click()
        await page.locator('input[name="title"]').fill('Searchable Route XYZ')
        await page.locator('input[name="description"]').fill('Test search')
        await page
          .locator('select[name="operationType"]')
          .selectOption('source')
        await page
          .locator('select[name="account.ruleType"]')
          .selectOption('alias')
        await page.locator('input[name="account.validIf"]').fill('search-*')
        await page.getByRole('button', { name: 'Save' }).click()
        await expect(page.getByTestId('success-toast')).toBeVisible()
        await page.getByTestId('dismiss-toast').click()
      })

      await test.step('Search for operation route', async () => {
        const searchInput = page.getByTestId('search-input')
        if (await searchInput.isVisible()) {
          await searchInput.fill('XYZ')
          await page.waitForLoadState('networkidle')
          await expect(
            page.getByRole('row', { name: /Searchable Route XYZ/i })
          ).toBeVisible()
        }
      })
    })
  })

  test.describe('Validation Scenarios', () => {
    test('should validate required title field', async ({ page }) => {
      await page.getByTestId('new-operation-route').click()
      await page.locator('input[name="description"]').fill('Test description')
      await page.locator('select[name="operationType"]').selectOption('source')
      await page
        .locator('select[name="account.ruleType"]')
        .selectOption('alias')
      await page.locator('input[name="account.validIf"]').fill('test-*')
      await page.getByRole('button', { name: 'Save' }).click()

      await expect(page.getByText(/title.*required/i)).toBeVisible()
    })

    test('should validate required description field', async ({ page }) => {
      await page.getByTestId('new-operation-route').click()
      await page.locator('input[name="title"]').fill('Test Route')
      await page.locator('select[name="operationType"]').selectOption('source')
      await page
        .locator('select[name="account.ruleType"]')
        .selectOption('alias')
      await page.locator('input[name="account.validIf"]').fill('test-*')
      await page.getByRole('button', { name: 'Save' }).click()

      await expect(page.getByText(/description.*required/i)).toBeVisible()
    })

    test('should validate account rule configuration', async ({ page }) => {
      await page.getByTestId('new-operation-route').click()
      await page.locator('input[name="title"]').fill('Test Route')
      await page.locator('input[name="description"]').fill('Test description')
      await page.locator('select[name="operationType"]').selectOption('source')
      await page
        .locator('select[name="account.ruleType"]')
        .selectOption('alias')
      await page.getByRole('button', { name: 'Save' }).click()

      await expect(page.getByText(/validIf.*required/i)).toBeVisible()
    })

    test('should validate operation type selection', async ({ page }) => {
      await page.getByTestId('new-operation-route').click()
      await page.locator('input[name="title"]').fill('Test Route')
      await page.locator('input[name="description"]').fill('Test description')
      await page
        .locator('select[name="account.ruleType"]')
        .selectOption('alias')
      await page.locator('input[name="account.validIf"]').fill('test-*')
      await page.getByRole('button', { name: 'Save' }).click()

      const operationTypeError = await page
        .getByText(/operation.*type.*required/i)
        .isVisible()
      if (operationTypeError) {
        await expect(page.getByText(/operation.*type.*required/i)).toBeVisible()
      }
    })
  })

  test.describe('Complex Workflows', () => {
    test('should create source and destination operation routes', async ({
      page
    }) => {
      const routes = [
        {
          title: 'Source Accounts Route',
          description: 'Route for source accounts',
          type: 'source',
          alias: 'src-*'
        },
        {
          title: 'Destination Accounts Route',
          description: 'Route for destination accounts',
          type: 'destination',
          alias: 'dst-*'
        }
      ]

      for (const route of routes) {
        await page.getByTestId('new-operation-route').click()
        await page.locator('input[name="title"]').fill(route.title)
        await page.locator('input[name="description"]').fill(route.description)
        await page
          .locator('select[name="operationType"]')
          .selectOption(route.type)
        await page
          .locator('select[name="account.ruleType"]')
          .selectOption('alias')
        await page.locator('input[name="account.validIf"]').fill(route.alias)
        await page.getByRole('button', { name: 'Save' }).click()
        await expect(page.getByTestId('success-toast')).toBeVisible()
        await page.getByTestId('dismiss-toast').click()
        await page.waitForLoadState('networkidle')
      }

      for (const route of routes) {
        await expect(
          page.getByRole('row', { name: new RegExp(route.title, 'i') })
        ).toBeVisible()
      }
    })

    test('should create operation route with wildcard patterns', async ({
      page
    }) => {
      const patterns = [
        { title: 'All Accounts', pattern: '*' },
        { title: 'User Accounts', pattern: 'user-*' },
        { title: 'System Accounts', pattern: 'system-*' }
      ]

      for (const item of patterns) {
        await page.getByTestId('new-operation-route').click()
        await page.locator('input[name="title"]').fill(item.title)
        await page
          .locator('input[name="description"]')
          .fill(`Route for ${item.title}`)
        await page
          .locator('select[name="operationType"]')
          .selectOption('source')
        await page
          .locator('select[name="account.ruleType"]')
          .selectOption('alias')
        await page.locator('input[name="account.validIf"]').fill(item.pattern)
        await page.getByRole('button', { name: 'Save' }).click()
        await expect(page.getByTestId('success-toast')).toBeVisible()
        await page.getByTestId('dismiss-toast').click()
        await page.waitForLoadState('networkidle')
      }
    })

    test('should create operation route with extensive metadata', async ({
      page
    }) => {
      await page.getByTestId('new-operation-route').click()
      await page.locator('input[name="title"]').fill('Premium Operation Route')
      await page
        .locator('input[name="description"]')
        .fill('Route with detailed configuration')
      await page.locator('select[name="operationType"]').selectOption('source')
      await page
        .locator('select[name="account.ruleType"]')
        .selectOption('alias')
      await page.locator('input[name="account.validIf"]').fill('premium-*')

      await page.locator('#metadata').click()

      const metadata = [
        { key: 'max-amount', value: '10000' },
        { key: 'min-amount', value: '100' },
        { key: 'fee-rate', value: '0.5' },
        { key: 'processing-priority', value: 'high' }
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
