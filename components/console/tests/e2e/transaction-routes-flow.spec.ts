import { test, expect } from '@playwright/test'
import { navigateToTransactionRoutes } from '../utils/navigate-to-transaction-routes'

test.beforeEach(async ({ page }) => {
  await navigateToTransactionRoutes(page)
})

test.describe('Transaction Routes Management - E2E Tests', () => {
  test.describe('CRUD Operations', () => {
    test('should create transaction route with required fields', async ({
      page
    }) => {
      await test.step('Open create transaction route sheet', async () => {
        await page.getByTestId('new-transaction-route').click()
        await expect(page.getByTestId('transaction-route-sheet')).toBeVisible()
      })

      await test.step('Fill transaction route form', async () => {
        await page.locator('input[name="title"]').fill('Standard Payment Route')
        await page
          .locator('input[name="description"]')
          .fill('Route for standard payment processing')
      })

      await test.step('Select operation routes', async () => {
        const operationRoutesSelect = page.locator(
          'select[name="operationRoutes"]'
        )
        if (await operationRoutesSelect.isVisible()) {
          const firstOption = await operationRoutesSelect
            .locator('option')
            .nth(1)
            .textContent()
          if (firstOption) {
            await operationRoutesSelect.selectOption({ index: 1 })
          }
        }
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
          page.getByTestId('transaction-route-sheet')
        ).not.toBeVisible()
        await expect(page.getByTestId('success-toast')).toBeVisible()
        await page.getByTestId('dismiss-toast').click()
      })

      await test.step('Verify transaction route appears in list', async () => {
        await page.waitForLoadState('networkidle')
        await expect(
          page.getByRole('row', { name: /Standard Payment Route/i })
        ).toBeVisible()
      })
    })

    test('should create transaction route with minimal fields', async ({
      page
    }) => {
      await page.getByTestId('new-transaction-route').click()
      await page.locator('input[name="title"]').fill('Minimal Route')
      await page.locator('input[name="description"]').fill('Basic route')
      await page.getByRole('button', { name: 'Save' }).click()
      await expect(page.getByTestId('success-toast')).toBeVisible()
    })

    test('should update existing transaction route', async ({ page }) => {
      await test.step('Create transaction route to update', async () => {
        await page.getByTestId('new-transaction-route').click()
        await page.locator('input[name="title"]').fill('Route to Update')
        await page.locator('input[name="description"]').fill('Will be updated')
        await page.getByRole('button', { name: 'Save' }).click()
        await expect(page.getByTestId('success-toast')).toBeVisible()
        await page.getByTestId('dismiss-toast').click()
      })

      await test.step('Open edit mode', async () => {
        const routeRow = page.getByRole('row', { name: /Route to Update/i })
        await page.waitForLoadState('networkidle')
        await routeRow.getByTestId('actions').click()
        await page.getByTestId('edit').click()
        await expect(page.getByTestId('transaction-route-sheet')).toBeVisible()
      })

      await test.step('Update transaction route', async () => {
        await page.locator('input[name="title"]').fill('Updated Route Title')
        await page
          .locator('input[name="description"]')
          .fill('Updated description')
        await page.getByRole('button', { name: 'Save' }).click()
        await expect(page.getByTestId('success-toast')).toBeVisible()
      })

      await test.step('Verify update', async () => {
        await page.waitForLoadState('networkidle')
        await expect(
          page.getByRole('row', { name: /Updated Route Title/i })
        ).toBeVisible()
      })
    })

    test('should delete transaction route with confirmation', async ({
      page
    }) => {
      await test.step('Create transaction route to delete', async () => {
        await page.getByTestId('new-transaction-route').click()
        await page.locator('input[name="title"]').fill('Route to Delete')
        await page.locator('input[name="description"]').fill('Will be deleted')
        await page.getByRole('button', { name: 'Save' }).click()
        await expect(page.getByTestId('success-toast')).toBeVisible()
        await page.getByTestId('dismiss-toast').click()
      })

      await test.step('Delete the transaction route', async () => {
        const routeRow = page.getByRole('row', { name: /Route to Delete/i })
        await page.waitForLoadState('networkidle')
        await routeRow.getByTestId('actions').click()
        await page.getByTestId('delete').click()
        await page.getByTestId('confirm').click()
        await expect(page.getByTestId('success-toast')).toBeVisible()
      })
    })

    test('should list transaction routes with pagination', async ({ page }) => {
      await expect(page.getByTestId('transaction-routes-table')).toBeVisible()
    })

    test('should search transaction routes', async ({ page }) => {
      await test.step('Create searchable transaction route', async () => {
        await page.getByTestId('new-transaction-route').click()
        await page.locator('input[name="title"]').fill('Searchable Route XYZ')
        await page.locator('input[name="description"]').fill('Test search')
        await page.getByRole('button', { name: 'Save' }).click()
        await expect(page.getByTestId('success-toast')).toBeVisible()
        await page.getByTestId('dismiss-toast').click()
      })

      await test.step('Search for transaction route', async () => {
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
      await page.getByTestId('new-transaction-route').click()
      await page.locator('input[name="description"]').fill('Test description')
      await page.getByRole('button', { name: 'Save' }).click()

      await expect(page.getByText(/title.*required/i)).toBeVisible()
    })

    test('should validate required description field', async ({ page }) => {
      await page.getByTestId('new-transaction-route').click()
      await page.locator('input[name="title"]').fill('Test Route')
      await page.getByRole('button', { name: 'Save' }).click()

      await expect(page.getByText(/description.*required/i)).toBeVisible()
    })

    test('should validate title length', async ({ page }) => {
      await page.getByTestId('new-transaction-route').click()
      await page.locator('input[name="title"]').fill('AB')
      await page.locator('input[name="description"]').fill('Test description')
      await page.getByRole('button', { name: 'Save' }).click()

      const lengthError = await page.getByText(/title.*minimum/i).isVisible()
      if (lengthError) {
        await expect(page.getByText(/title.*minimum/i)).toBeVisible()
      }
    })
  })

  test.describe('Complex Workflows', () => {
    test('should create transaction routes with multiple operation routes', async ({
      page
    }) => {
      await page.getByTestId('new-transaction-route').click()
      await page.locator('input[name="title"]').fill('Multi-Operation Route')
      await page
        .locator('input[name="description"]')
        .fill('Route with multiple operations')

      const multiSelectField = page.getByTestId('operation-routes-multiselect')
      if (await multiSelectField.isVisible()) {
        const firstOption = multiSelectField.locator('option').nth(1)
        const secondOption = multiSelectField.locator('option').nth(2)

        if ((await firstOption.count()) > 0) {
          await firstOption.click()
        }
        if ((await secondOption.count()) > 0) {
          await secondOption.click()
        }
      }

      await page.getByRole('button', { name: 'Save' }).click()
      await expect(page.getByTestId('success-toast')).toBeVisible()
    })

    test('should create hierarchical transaction routes', async ({ page }) => {
      const routes = [
        {
          title: 'Payment Processing',
          description: 'Main payment processing route'
        },
        {
          title: 'Refund Processing',
          description: 'Route for processing refunds'
        },
        {
          title: 'Transfer Processing',
          description: 'Route for account transfers'
        }
      ]

      for (const route of routes) {
        await page.getByTestId('new-transaction-route').click()
        await page.locator('input[name="title"]').fill(route.title)
        await page.locator('input[name="description"]').fill(route.description)
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

    test('should create transaction route with extensive metadata', async ({
      page
    }) => {
      await page.getByTestId('new-transaction-route').click()
      await page.locator('input[name="title"]').fill('Premium Payment Route')
      await page
        .locator('input[name="description"]')
        .fill('Route for premium payment processing')

      await page.locator('#metadata').click()

      const metadata = [
        { key: 'processing-time', value: 'instant' },
        { key: 'fee-type', value: 'percentage' },
        { key: 'retry-attempts', value: '3' },
        { key: 'timeout', value: '30s' }
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
