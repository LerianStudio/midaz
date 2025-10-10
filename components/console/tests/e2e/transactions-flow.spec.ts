import { test, expect } from '@playwright/test'
import {
  fillSimpleTransaction,
  fillComplexTransaction
} from '../utils/transactions'
import {
  SIMPLE_TRANSACTION_FORM_DATA,
  COMPLEX_TRANSACTION_FORM_DATA
} from '../fixtures/transactions'

test.describe('Transactions Management - E2E Tests', () => {
  test.describe('Navigate to Transactions from the Home', () => {
    test('should navigate to transactions page from sidebar', async ({
      page
    }) => {
      // Start at home page
      await page.goto('/', { waitUntil: 'domcontentloaded' })

      // Click on Transactions in the sidebar using text selector
      await page.getByRole('link', { name: /transactions/i }).click()

      // Wait for navigation
      await page.waitForURL(/\/transactions/, { timeout: 10000 })

      // Verify we're on the transactions page
      await expect(page.getByTestId('title')).toBeVisible({ timeout: 15000 })

      // Verify new transaction button is present
      await expect(page.getByTestId('new-transaction')).toBeVisible()
    })
  })

  test.describe('Tests for List Transactions', () => {
    test.beforeEach(async ({ page }) => {
      // Navigate to transactions page before each test
      await page.goto('/transactions', { waitUntil: 'domcontentloaded' })
      await page.waitForTimeout(2000) // Wait for data to load
    })

    test('should display transactions table with correct columns', async ({
      page
    }) => {
      // Wait for table to appear (environment is pre-seeded)
      await page
        .getByTestId('transactions-table')
        .waitFor({ state: 'visible', timeout: 10000 })

      // Verify all required table columns
      await expect(
        page.getByRole('columnheader', { name: /^data$/i })
      ).toBeVisible()
      await expect(
        page.getByRole('columnheader', { name: /^id$/i })
      ).toBeVisible()
      await expect(
        page.getByRole('columnheader', { name: /source/i })
      ).toBeVisible()
      await expect(
        page.getByRole('columnheader', { name: /destination/i })
      ).toBeVisible()
      await expect(
        page.getByRole('columnheader', { name: /status/i })
      ).toBeVisible()
      await expect(
        page.getByRole('columnheader', { name: /value/i })
      ).toBeVisible()
      await expect(
        page.getByRole('columnheader', { name: /actions/i })
      ).toBeVisible()
    })

    test('should display transaction rows with correct data', async ({
      page
    }) => {
      // Wait for table to appear (environment is pre-seeded)
      await page
        .getByTestId('transactions-table')
        .waitFor({ state: 'visible', timeout: 10000 })

      // Verify transaction rows exist
      const transactionRows = page.getByTestId('transaction-row')
      const rowCount = await transactionRows.count()
      expect(rowCount).toBeGreaterThan(0)

      // Verify first row has all expected cells
      const firstRow = transactionRows.first()
      const cells = firstRow.locator('td')
      const cellCount = await cells.count()

      // Should have 7 columns: Date, ID, Source, Destination, Status, Value, Actions
      expect(cellCount).toBe(7)
    })

    test('should display transaction status badge', async ({ page }) => {
      // Wait for table to appear (environment is pre-seeded)
      await page
        .getByTestId('transactions-table')
        .waitFor({ state: 'visible', timeout: 10000 })

      const firstRow = page.getByTestId('transaction-row').first()
      await firstRow.waitFor({ state: 'visible', timeout: 5000 })

      // Status badge should be visible (can be Approved or Canceled)
      const badge = firstRow.locator('[class*="badge"]').first()
      await expect(badge).toBeVisible()
    })

    test('should open actions dropdown menu', async ({ page }) => {
      // Wait for table to appear (environment is pre-seeded)
      await page
        .getByTestId('transactions-table')
        .waitFor({ state: 'visible', timeout: 10000 })

      const actionsButton = page.getByTestId('actions').first()
      await actionsButton.waitFor({ state: 'visible', timeout: 5000 })
      await actionsButton.click()

      // Verify "See details" option appears
      await expect(
        page.getByRole('menuitem', { name: /see details/i })
      ).toBeVisible()
    })

    test('should display pagination controls', async ({ page }) => {
      // Wait for table to appear (environment is pre-seeded)
      await page
        .getByTestId('transactions-table')
        .waitFor({ state: 'visible', timeout: 10000 })

      // Verify showing count text is visible
      await expect(page.getByText(/showing.*transaction/i)).toBeVisible()
    })
  })

  test.describe('Create Transaction', () => {
    test.beforeEach(async ({ page }) => {
      // Navigate to transactions page before each test
      await page.goto('/transactions', { waitUntil: 'domcontentloaded' })
      await page
        .getByTestId('transactions-table')
        .waitFor({ state: 'visible', timeout: 10000 })
    })

    test('should open transaction mode selection modal', async ({ page }) => {
      // Click the "New Transaction" button
      await page.getByTestId('new-transaction').click()

      // Verify modal is visible
      await expect(page.getByTestId('transaction-mode-modal')).toBeVisible()

      // Verify modal title
      await expect(
        page.getByRole('heading', { name: /new transaction/i })
      ).toBeVisible()

      // Verify mode selection description
      await expect(
        page.getByText(/select the type of transaction you want to create/i)
      ).toBeVisible()
    })

    test('should display both Simple and Advanced mode options', async ({
      page
    }) => {
      await page.getByTestId('new-transaction').click()

      // Wait for modal and its content to be visible
      await page
        .getByTestId('transaction-mode-modal')
        .waitFor({ state: 'visible', timeout: 10000 })

      // Wait for Simple mode button to appear
      const simpleModeButton = page.getByTestId('simple-mode')
      await simpleModeButton.waitFor({ state: 'visible', timeout: 5000 })
      await expect(simpleModeButton).toContainText(/simple 1:1/i)

      // Verify Advanced mode button
      const advancedModeButton = page.getByTestId('advanced-mode')
      await expect(advancedModeButton).toBeVisible()
      await expect(advancedModeButton).toContainText(/complex n:n/i)
    })

    test('should navigate to Simple transaction form when selected', async ({
      page
    }) => {
      await page.getByTestId('new-transaction').click()
      await expect(page.getByTestId('transaction-mode-modal')).toBeVisible()

      // Click Simple mode
      await page.getByTestId('simple-mode').click()

      // Wait for navigation to create page
      await page.waitForURL(/\/transactions\/create/, { timeout: 5000 })

      // Verify we're on the create page
      await expect(page.getByTestId('transaction-form-title')).toBeVisible({
        timeout: 10000
      })
    })

    test('should fill and submit Simple transaction form', async ({ page }) => {
      // Open modal and select Simple mode
      await page.getByTestId('new-transaction').click()
      await page
        .getByTestId('transaction-mode-modal')
        .waitFor({ state: 'visible', timeout: 10000 })
      await page.getByTestId('simple-mode').click()

      // Wait for form to load
      await page.waitForURL(/\/transactions\/create/, { timeout: 5000 })
      await expect(page.getByTestId('transaction-form-title')).toBeVisible({
        timeout: 10000
      })

      // Fill the simple transaction form
      await fillSimpleTransaction(
        page,
        SIMPLE_TRANSACTION_FORM_DATA.E2E_BRL_DEPOSIT
      )

      // Click the Review button
      await page.getByTestId('transaction-review-button').click()

      // Verify we're on the review step (same route, different view)
      await expect(page.getByTestId('transaction-review-title')).toBeVisible({
        timeout: 10000
      })

      // Take screenshot of review page
      await page.screenshot({ path: 'test-transaction-review.png' })

      // Submit the transaction
      await page.getByTestId('transaction-submit-button').click()

      // Wait for redirect to transaction detail page (contains UUID in URL)
      await page.waitForURL(/\/transactions\/[a-f0-9-]{36}/, { timeout: 15000 })

      // Verify we're on the transaction detail page
      await expect(page).toHaveURL(/\/transactions\/[a-f0-9-]{36}/)
    })

    test('should fill and submit Simple transaction form with metadata', async ({
      page
    }) => {
      // Open modal and select Simple mode
      await page.getByTestId('new-transaction').click()
      await page
        .getByTestId('transaction-mode-modal')
        .waitFor({ state: 'visible', timeout: 10000 })
      await page.getByTestId('simple-mode').click()

      // Wait for form to load
      await page.waitForURL(/\/transactions\/create/, { timeout: 5000 })
      await expect(page.getByTestId('transaction-form-title')).toBeVisible({
        timeout: 10000
      })

      // Fill the simple transaction form with metadata
      await fillSimpleTransaction(
        page,
        SIMPLE_TRANSACTION_FORM_DATA.E2E_BRL_DEPOSIT_WITH_METADATA
      )

      // Click the Review button
      await page.getByTestId('transaction-review-button').click()

      // Verify we're on the review step
      await expect(page.getByTestId('transaction-review-title')).toBeVisible({
        timeout: 10000
      })

      // Take screenshot of review page with metadata
      await page.screenshot({
        path: 'test-transaction-review-with-metadata.png'
      })

      // Submit the transaction
      await page.getByTestId('transaction-submit-button').click()

      // Wait for redirect to transaction detail page
      await page.waitForURL(/\/transactions\/[a-f0-9-]{36}/, { timeout: 15000 })

      // Verify we're on the transaction detail page
      await expect(page).toHaveURL(/\/transactions\/[a-f0-9-]{36}/)
    })

    test('should fill and submit Complex transaction form', async ({
      page
    }) => {
      // Open modal and select Complex mode
      await page.getByTestId('new-transaction').click()
      await page
        .getByTestId('transaction-mode-modal')
        .waitFor({ state: 'visible', timeout: 10000 })
      await page.getByTestId('advanced-mode').click()

      // Wait for form to load
      await page.waitForURL(/\/transactions\/create/, { timeout: 5000 })
      await expect(page.getByTestId('transaction-form-title')).toBeVisible({
        timeout: 10000
      })

      // Fill the complex transaction form
      await fillComplexTransaction(
        page,
        COMPLEX_TRANSACTION_FORM_DATA.E2E_MULTI_ACCOUNT
      )

      // Click the Review button
      await page.getByTestId('transaction-review-button').click()

      // Verify we're on the review step
      await expect(page.getByTestId('transaction-review-title')).toBeVisible({
        timeout: 10000
      })

      // Take screenshot of review page
      await page.screenshot({
        path: 'test-complex-transaction-review.png'
      })

      // Submit the transaction
      await page.getByTestId('transaction-submit-button').click()

      // Wait for redirect to transaction detail page
      await page.waitForURL(/\/transactions\/[a-f0-9-]{36}/, { timeout: 15000 })

      // Verify we're on the transaction detail page
      await expect(page).toHaveURL(/\/transactions\/[a-f0-9-]{36}/)
    })
  })
})
