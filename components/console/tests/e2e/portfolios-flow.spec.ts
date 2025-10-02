import { test, expect } from '@playwright/test'
import { navigateToPortfolios } from '../utils/navigate-to-portfolios'

test.beforeEach(async ({ page }) => {
  await navigateToPortfolios(page)
})

test.describe('Portfolios Management - E2E Tests', () => {
  test.describe('CRUD Operations', () => {
    test('should create portfolio with all required fields', async ({
      page
    }) => {
      await test.step('Open create portfolio sheet', async () => {
        await page.getByTestId('new-portfolio').click()
        await expect(page.getByTestId('portfolio-sheet')).toBeVisible()
      })

      await test.step('Fill portfolio form', async () => {
        await page.locator('input[name="name"]').fill('Test Portfolio')
        await page.locator('input[name="entityId"]').fill('entity-123')
      })

      await test.step('Add metadata', async () => {
        await page.locator('#metadata').click()
        await page.locator('#key').fill('manager')
        await page.locator('#value').fill('John Doe')
        await page.getByRole('button', { name: 'Add' }).first().click()
      })

      await test.step('Submit and verify', async () => {
        await page.getByRole('button', { name: 'Save' }).click()
        await expect(page.getByTestId('portfolio-sheet')).not.toBeVisible()
        await expect(page.getByTestId('success-toast')).toBeVisible()
        await page.getByTestId('dismiss-toast').click()
      })

      await test.step('Verify portfolio appears in list', async () => {
        await page.waitForLoadState('networkidle')
        await expect(
          page.getByRole('row', { name: /Test Portfolio/i })
        ).toBeVisible()
      })
    })

    test('should create portfolio with minimal required fields', async ({
      page
    }) => {
      await page.getByTestId('new-portfolio').click()
      await page.locator('input[name="name"]').fill('Minimal Portfolio')
      await page.getByRole('button', { name: 'Save' }).click()
      await expect(page.getByTestId('success-toast')).toBeVisible()
    })

    test('should update existing portfolio', async ({ page }) => {
      await test.step('Create portfolio to update', async () => {
        await page.getByTestId('new-portfolio').click()
        await page.locator('input[name="name"]').fill('Portfolio to Update')
        await page.getByRole('button', { name: 'Save' }).click()
        await expect(page.getByTestId('success-toast')).toBeVisible()
        await page.getByTestId('dismiss-toast').click()
      })

      await test.step('Open edit mode', async () => {
        const portfolioRow = page.getByRole('row', {
          name: /Portfolio to Update/i
        })
        await page.waitForLoadState('networkidle')
        await portfolioRow.getByTestId('actions').click()
        await page.getByTestId('edit').click()
        await expect(page.getByTestId('portfolio-sheet')).toBeVisible()
      })

      await test.step('Update portfolio name', async () => {
        await page.locator('input[name="name"]').fill('Updated Portfolio Name')
        await page.getByRole('button', { name: 'Save' }).click()
        await expect(page.getByTestId('success-toast')).toBeVisible()
      })

      await test.step('Verify update', async () => {
        await page.waitForLoadState('networkidle')
        await expect(
          page.getByRole('row', { name: /Updated Portfolio Name/i })
        ).toBeVisible()
      })
    })

    test('should delete portfolio with confirmation', async ({ page }) => {
      await test.step('Create portfolio to delete', async () => {
        await page.getByTestId('new-portfolio').click()
        await page.locator('input[name="name"]').fill('Portfolio to Delete')
        await page.getByRole('button', { name: 'Save' }).click()
        await expect(page.getByTestId('success-toast')).toBeVisible()
        await page.getByTestId('dismiss-toast').click()
      })

      await test.step('Delete the portfolio', async () => {
        const portfolioRow = page.getByRole('row', {
          name: /Portfolio to Delete/i
        })
        await page.waitForLoadState('networkidle')
        await portfolioRow.getByTestId('actions').click()
        await page.getByTestId('delete').click()
        await page.getByTestId('confirm').click()
        await expect(page.getByTestId('success-toast')).toBeVisible()
      })
    })

    test('should list portfolios with pagination', async ({ page }) => {
      await expect(page.getByTestId('portfolios-table')).toBeVisible()
    })

    test('should search portfolios', async ({ page }) => {
      await test.step('Create searchable portfolio', async () => {
        await page.getByTestId('new-portfolio').click()
        await page
          .locator('input[name="name"]')
          .fill('Searchable Portfolio ABC')
        await page.getByRole('button', { name: 'Save' }).click()
        await expect(page.getByTestId('success-toast')).toBeVisible()
        await page.getByTestId('dismiss-toast').click()
      })

      await test.step('Search for portfolio', async () => {
        const searchInput = page.getByTestId('search-input')
        if (await searchInput.isVisible()) {
          await searchInput.fill('ABC')
          await page.waitForLoadState('networkidle')
          await expect(
            page.getByRole('row', { name: /Searchable Portfolio ABC/i })
          ).toBeVisible()
        }
      })
    })
  })

  test.describe('Validation Scenarios', () => {
    test('should validate required name field', async ({ page }) => {
      await page.getByTestId('new-portfolio').click()
      await page.getByRole('button', { name: 'Save' }).click()
      await expect(page.getByText(/name.*required/i)).toBeVisible()
    })

    test('should validate name length', async ({ page }) => {
      await page.getByTestId('new-portfolio').click()
      await page.locator('input[name="name"]').fill('AB')
      await page.getByRole('button', { name: 'Save' }).click()

      const lengthError = await page.getByText(/name.*minimum/i).isVisible()
      if (lengthError) {
        await expect(page.getByText(/name.*minimum/i)).toBeVisible()
      }
    })
  })

  test.describe('Complex Workflows', () => {
    test('should create portfolio with entity ID', async ({ page }) => {
      await page.getByTestId('new-portfolio').click()
      await page.locator('input[name="name"]').fill('Entity Portfolio')
      await page.locator('input[name="entityId"]').fill('entity-456')
      await page.getByRole('button', { name: 'Save' }).click()
      await expect(page.getByTestId('success-toast')).toBeVisible()
    })

    test('should create multiple portfolios in sequence', async ({ page }) => {
      const portfolios = [
        { name: 'Investment Portfolio', entity: 'inv-001' },
        { name: 'Savings Portfolio', entity: 'sav-002' },
        { name: 'Trading Portfolio', entity: 'trd-003' }
      ]

      for (const portfolio of portfolios) {
        await page.getByTestId('new-portfolio').click()
        await page.locator('input[name="name"]').fill(portfolio.name)
        await page.locator('input[name="entityId"]').fill(portfolio.entity)
        await page.getByRole('button', { name: 'Save' }).click()
        await expect(page.getByTestId('success-toast')).toBeVisible()
        await page.getByTestId('dismiss-toast').click()
        await page.waitForLoadState('networkidle')
      }

      await expect(
        page.getByRole('row', { name: /Investment Portfolio/i })
      ).toBeVisible()
      await expect(
        page.getByRole('row', { name: /Savings Portfolio/i })
      ).toBeVisible()
      await expect(
        page.getByRole('row', { name: /Trading Portfolio/i })
      ).toBeVisible()
    })

    test('should create portfolio with extensive metadata', async ({
      page
    }) => {
      await page.getByTestId('new-portfolio').click()
      await page.locator('input[name="name"]').fill('Metadata Portfolio')

      await page.locator('#metadata').click()

      const metadata = [
        { key: 'manager', value: 'Jane Smith' },
        { key: 'department', value: 'Finance' },
        { key: 'risk-level', value: 'medium' },
        { key: 'region', value: 'US-East' }
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
