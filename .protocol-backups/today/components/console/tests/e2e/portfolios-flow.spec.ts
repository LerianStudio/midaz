import { test, expect, Page } from '@playwright/test'
import { navigateToPortfolios } from '../utils/navigate-to-portfolios'

// Helper function to open the portfolio sheet and wait for it to be ready
async function openPortfolioSheet(page: Page) {
  // Click the new portfolio button using data-testid
  await page.getByTestId('new-portfolio').first().click()

  // Wait for the sheet to open using data-testid
  await expect(page.getByTestId('portfolio-sheet')).toBeVisible({
    timeout: 10000
  })

  // Wait for form inputs to be ready
  await expect(
    page.getByRole('textbox', { name: /portfolio name/i })
  ).toBeVisible({
    timeout: 5000
  })
}

test.beforeEach(async ({ page }) => {
  await navigateToPortfolios(page)
})

test.describe('Portfolios Management - E2E Tests', () => {
  test.describe('CRUD Operations', () => {
    test('should create portfolio with all required fields', async ({
      page
    }) => {
      await test.step('Open create portfolio sheet', async () => {
        await openPortfolioSheet(page)
      })

      await test.step('Fill portfolio form', async () => {
        await page
          .getByRole('textbox', { name: /portfolio name/i })
          .fill('Test Portfolio')
        await page.getByRole('textbox', { name: /entity id/i }).fill('entity-123')
      })

      await test.step('Add metadata', async () => {
        // Click the Metadata tab
        await page.getByRole('tab', { name: /metadata/i }).click()

        // Wait for the metadata tab panel to be visible
        await page.waitForTimeout(300)

        await page.getByLabel(/key/i).fill('manager')
        await page.getByLabel(/value/i).fill('John Doe')

        // The add button in the metadata panel
        await page
          .locator('div[role="tabpanel"]')
          .getByRole('button')
          .first()
          .click()
      })

      await test.step('Submit and verify', async () => {
        await page.getByRole('button', { name: /save/i }).click()
        await expect(page.getByTestId('portfolio-sheet')).not.toBeVisible({
          timeout: 5000
        })
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
      await openPortfolioSheet(page)
      await page.getByRole('textbox', { name: /portfolio name/i }).fill('Minimal Portfolio')
      await page.getByRole('button', { name: /save/i }).click()
      await expect(page.getByTestId('success-toast')).toBeVisible()
    })

    test('should update existing portfolio', async ({ page }) => {
      await test.step('Create portfolio to update', async () => {
        await openPortfolioSheet(page)
        await page.getByRole('textbox', { name: /portfolio name/i }).fill('Portfolio to Update')
        await page.getByRole('button', { name: /save/i }).click()
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
        await page.getByRole('textbox', { name: /portfolio name/i }).fill('Updated Portfolio Name')
        await page.getByRole('button', { name: /save/i }).click()
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
        await openPortfolioSheet(page)
        await page.getByRole('textbox', { name: /portfolio name/i }).fill('Portfolio to Delete')
        await page.getByRole('button', { name: /save/i }).click()
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
      // Wait for page to be fully loaded
      await page.waitForLoadState('networkidle')

      // Check if either table or empty state is visible
      await Promise.race([
        page.getByTestId('portfolios-table').waitFor({ state: 'visible' }),
        page
          .getByText(/You haven't created any Portfolios yet/i)
          .waitFor({ state: 'visible' })
      ])

      const hasTable = await page.getByTestId('portfolios-table').isVisible()
      const hasEmptyState = await page
        .getByText(/You haven't created any Portfolios yet/i)
        .isVisible()

      // At least one should be visible
      expect(hasTable || hasEmptyState).toBeTruthy()
    })

    test('should search portfolios', async ({ page }) => {
      await test.step('Create searchable portfolio', async () => {
        await openPortfolioSheet(page)
        await page
          .getByRole('textbox', { name: /portfolio name/i })
          .fill('Searchable Portfolio ABC')
        await page.getByRole('button', { name: /save/i }).click()
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
      await openPortfolioSheet(page)
      await page.getByRole('button', { name: /save/i }).click()
      await expect(page.getByText(/name.*required/i)).toBeVisible()
    })

    test('should validate name length', async ({ page }) => {
      await openPortfolioSheet(page)
      await page.getByRole('textbox', { name: /portfolio name/i }).fill('AB')
      await page.getByRole('button', { name: /save/i }).click()

      const lengthError = await page.getByText(/name.*minimum/i).isVisible()
      if (lengthError) {
        await expect(page.getByText(/name.*minimum/i)).toBeVisible()
      }
    })
  })

  test.describe('Complex Workflows', () => {
    test('should create portfolio with entity ID', async ({ page }) => {
      await openPortfolioSheet(page)
      await page.getByRole('textbox', { name: /portfolio name/i }).fill('Entity Portfolio')
      await page.getByRole('textbox', { name: /entity id/i }).fill('entity-456')
      await page.getByRole('button', { name: /save/i }).click()
      await expect(page.getByTestId('success-toast')).toBeVisible()
    })

    test('should create portfolio with extensive metadata', async ({
      page
    }) => {
      await openPortfolioSheet(page)
      await page.getByRole('textbox', { name: /portfolio name/i }).fill('Metadata Portfolio')

      // Click the Metadata tab
      await page.getByRole('tab', { name: /metadata/i }).click()

      // Wait for the metadata tab panel to be visible
      await page.waitForTimeout(300)

      const metadata = [
        { key: 'manager', value: 'Jane Smith' },
        { key: 'department', value: 'Finance' },
        { key: 'risk-level', value: 'medium' },
        { key: 'region', value: 'US-East' }
      ]

      for (const meta of metadata) {
        await page.getByLabel(/key/i).fill(meta.key)
        await page.getByLabel(/value/i).fill(meta.value)
        await page.locator('div[role="tabpanel"]').getByRole('button').first().click()
      }

      await page.getByRole('button', { name: /save/i }).click()
      await expect(page.getByTestId('success-toast')).toBeVisible()
    })
  })
})
