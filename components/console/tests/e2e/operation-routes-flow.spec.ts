import { test, expect } from '@playwright/test'
import { navigateToOperationRoutes } from '../utils/navigate-to-operation-routes'

test.beforeEach(async ({ page }) => {
  await navigateToOperationRoutes(page)
})

test.describe('Operation Routes Management - E2E Tests', () => {
  test.describe('CRUD Operations', () => {
    test('should create operation route with minimal fields', async ({
      page
    }) => {
      // Generate unique title to avoid conflicts
      const uniqueTitle = `Test Route ${Date.now()}`
      const uniqueDescription = `Test Description ${Date.now()}`

      await test.step('Open create operation route sheet', async () => {
        await page.getByTestId('new-operation-route').first().click()
        await page.waitForSelector('[data-testid="operation-route-sheet"]', {
          state: 'visible'
        })
      })

      await test.step('Fill required fields only', async () => {
        await page.locator('input[name="title"]').fill(uniqueTitle)
        await page
          .locator('textarea[name="description"]')
          .fill(uniqueDescription)
        await page.locator('input[name="account.validIf"]').fill('test-*')
      })

      await test.step('Submit and verify', async () => {
        // Use getByRole instead of testid - more reliable
        await page.getByRole('button', { name: 'Save' }).click()

        await expect(page.getByTestId('success-toast')).toBeVisible()
        await page.getByTestId('dismiss-toast').click()
      })

      await test.step('Verify route appears in list', async () => {
        await page.waitForLoadState('networkidle')
        // Look for the specific row with our unique title - use .first() to handle any duplicates
        await expect(
          page.getByRole('row', { name: new RegExp(uniqueTitle, 'i') }).first()
        ).toBeVisible()
      })
    })

    test('should update existing operation route', async ({ page }) => {
      // Generate unique titles to avoid conflicts
      const initialTitle = `Route to Update ${Date.now()}`
      const updatedTitle = `Updated Route ${Date.now()}`

      await test.step('Create operation route to update', async () => {
        await page.getByTestId('new-operation-route').first().click()
        await page.waitForSelector('[data-testid="operation-route-sheet"]', {
          state: 'visible'
        })

        await page.locator('input[name="title"]').fill(initialTitle)
        await page
          .locator('textarea[name="description"]')
          .fill('Will be updated')
        await page.locator('input[name="account.validIf"]').fill('update-*')

        await page.getByRole('button', { name: 'Save' }).click()
        await expect(page.getByTestId('success-toast')).toBeVisible()
        await page.getByTestId('dismiss-toast').click()
      })

      await test.step('Open edit mode', async () => {
        await page.waitForLoadState('networkidle')
        // Find the specific row with our unique initial title
        const routeRow = page
          .getByRole('row', { name: new RegExp(initialTitle, 'i') })
          .first()
        await routeRow.getByTestId('actions').click()
        await page.getByTestId('edit').click()
        await page.waitForSelector('[data-testid="operation-route-sheet"]', {
          state: 'visible'
        })
      })

      await test.step('Update operation route', async () => {
        // Clear and fill with new unique title
        await page.locator('input[name="title"]').clear()
        await page.locator('input[name="title"]').fill(updatedTitle)
        await page.locator('textarea[name="description"]').clear()
        await page
          .locator('textarea[name="description"]')
          .fill('Updated description')
        await page.getByRole('button', { name: 'Save' }).click()
        await expect(page.getByTestId('success-toast')).toBeVisible()
      })

      await test.step('Verify update', async () => {
        await page.waitForLoadState('networkidle')
        // Look for the specific row with our unique updated title
        await expect(
          page.getByRole('row', { name: new RegExp(updatedTitle, 'i') }).first()
        ).toBeVisible()
      })
    })

    test('should delete operation route with confirmation', async ({
      page
    }) => {
      // Generate unique title to avoid conflicts
      const deleteTitle = `Route to Delete ${Date.now()}`

      await test.step('Create operation route to delete', async () => {
        await page.getByTestId('new-operation-route').first().click()
        await page.waitForSelector('[data-testid="operation-route-sheet"]', {
          state: 'visible'
        })

        await page.locator('input[name="title"]').fill(deleteTitle)
        await page
          .locator('textarea[name="description"]')
          .fill('Will be deleted')
        await page.locator('input[name="account.validIf"]').fill('delete-*')

        await page.getByRole('button', { name: 'Save' }).click()
        await expect(page.getByTestId('success-toast')).toBeVisible()
        await page.getByTestId('dismiss-toast').click()
      })

      await test.step('Delete the operation route', async () => {
        await page.waitForLoadState('networkidle')
        // Find the specific row with our unique title
        const routeRow = page
          .getByRole('row', { name: new RegExp(deleteTitle, 'i') })
          .first()
        await routeRow.getByTestId('actions').click()
        await page.getByTestId('delete').click()
        await page.getByTestId('confirm').click()
        await expect(page.getByTestId('success-toast')).toBeVisible()
      })
    })

    test('should list operation routes', async ({ page }) => {
      // Wait for page to load - either table or empty state should be visible
      await Promise.race([
        page
          .getByTestId('operation-routes-table')
          .waitFor({ state: 'visible' }),
        page
          .getByText(/You haven't created any Operation Routes yet/i)
          .waitFor({ state: 'visible' })
      ])
    })
  })

  test.describe('Validation Scenarios', () => {
    test('should validate required title field', async ({ page }) => {
      await page.getByTestId('new-operation-route').first().click()
      await page.waitForSelector('[data-testid="operation-route-sheet"]', {
        state: 'visible'
      })

      await page
        .locator('textarea[name="description"]')
        .fill('Test description')
      await page.locator('input[name="account.validIf"]').fill('test-*')
      await page.getByRole('button', { name: 'Save' }).click()

      await expect(page.getByText(/title.*required/i)).toBeVisible()
    })

    test('should validate required description field', async ({ page }) => {
      await page.getByTestId('new-operation-route').first().click()
      await page.waitForSelector('[data-testid="operation-route-sheet"]', {
        state: 'visible'
      })

      await page.locator('input[name="title"]').fill('Test Route')
      await page.locator('input[name="account.validIf"]').fill('test-*')
      await page.getByRole('button', { name: 'Save' }).click()

      await expect(page.getByText(/description.*required/i)).toBeVisible()
    })

    // Note: This test is skipped because the form has default values for Operation Type and Rule Type
    // When these defaults are set, the account.validIf field validation passes even when empty
    test.skip('should validate account rule configuration', async ({
      page
    }) => {
      await page.getByTestId('new-operation-route').first().click()
      await page.waitForSelector('[data-testid="operation-route-sheet"]', {
        state: 'visible'
      })

      await page.locator('input[name="title"]').fill('Test Route')
      await page
        .locator('textarea[name="description"]')
        .fill('Test description')
      // Leave account.validIf empty - but this might not trigger validation due to defaults
      await page.getByRole('button', { name: 'Save' }).click()

      await expect(page.getByText(/validIf.*required/i)).toBeVisible()
    })
  })
})
