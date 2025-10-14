import { test, expect } from '@playwright/test'
import {
  fillSimpleTransaction,
  fillComplexTransaction
} from '../utils/transactions'
import {
  SIMPLE_TRANSACTION_FORM_DATA,
  COMPLEX_TRANSACTION_FORM_DATA
} from '../fixtures/transactions'
import { createTransaction } from '../setup/setup-transaction'
import { ASSETS } from '../fixtures/assets'
import { ACCOUNTS } from '../fixtures/accounts'
import { resetAccountBalance } from '../utils/accounts'

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
      const table = page.getByTestId('transactions-table')
      await table.waitFor({ state: 'visible', timeout: 10000 })

      // Verify all required table columns exist in the header
      const tableHeader = table.locator('thead')
      await expect(tableHeader).toBeVisible()

      // Verify column headers by text content
      await expect(tableHeader.getByText('Data', { exact: true })).toBeVisible()
      await expect(tableHeader.getByText('ID', { exact: true })).toBeVisible()
      await expect(
        tableHeader.getByText('Source', { exact: true })
      ).toBeVisible()
      await expect(
        tableHeader.getByText('Destination', { exact: true })
      ).toBeVisible()
      await expect(
        tableHeader.getByText('Status', { exact: true })
      ).toBeVisible()
      await expect(
        tableHeader.getByText('Value', { exact: true })
      ).toBeVisible()
      await expect(
        tableHeader.getByText('Actions', { exact: true })
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
      // Look for the status text in the Status column
      const statusCell = firstRow.locator('td').nth(4) // Status is the 5th column (0-indexed: 4)
      await expect(statusCell).toBeVisible()

      // Verify status text is either "Approved" or "Canceled"
      const statusText = await statusCell.textContent()
      expect(statusText).toMatch(/Approved|Canceled/)
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
    // Reset all BRL accounts to exactly 100 BRL before running creation tests
    test.beforeAll(async () => {
      // eslint-disable-next-line no-console
      console.log('Resetting account balances to 100 BRL...')

      const brlAccounts = [
        ACCOUNTS.BRL_ACCOUNT,
        ACCOUNTS.BRL_ACCOUNT_2,
        ACCOUNTS.BRL_ACCOUNT_3,
        ACCOUNTS.BRL_ACCOUNT_4,
        ACCOUNTS.BRL_ACCOUNT_5
      ]

      for (const account of brlAccounts) {
        await resetAccountBalance(account.alias, ASSETS.BRL.code, 100)
      }

      // eslint-disable-next-line no-console
      console.log('✓ All account balances reset to 100 BRL')
    })

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
        page.getByTestId('transaction-mode-modal-title')
      ).toBeVisible()

      // Verify mode selection description
      await expect(
        page.getByTestId('transaction-mode-modal-description')
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

      // Verify Advanced mode button
      const advancedModeButton = page.getByTestId('advanced-mode')
      await expect(advancedModeButton).toBeVisible()
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

      // Submit the transaction
      await page.getByTestId('transaction-submit-button').click()

      // Wait for redirect to transaction detail page
      await page.waitForURL(/\/transactions\/[a-f0-9-]{36}/, { timeout: 15000 })

      // Verify we're on the transaction detail page
      await expect(page).toHaveURL(/\/transactions\/[a-f0-9-]{36}/)
    })

    test('should fill and submit Complex transaction form (1:1)', async ({
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

      // Fill the complex transaction form (simple 1:1 transfer)
      await fillComplexTransaction(
        page,
        COMPLEX_TRANSACTION_FORM_DATA.E2E_SIMPLE_TRANSFER
      )

      // Click the Review button
      await page.getByTestId('transaction-review-button').click()

      // Verify we're on the review step
      await expect(page.getByTestId('transaction-review-title')).toBeVisible({
        timeout: 10000
      })

      // Submit the transaction
      await page.getByTestId('transaction-submit-button').click()

      // Wait for redirect to transaction detail page
      await page.waitForURL(/\/transactions\/[a-f0-9-]{36}/, { timeout: 15000 })

      // Verify we're on the transaction detail page
      await expect(page).toHaveURL(/\/transactions\/[a-f0-9-]{36}/)
    })

    test('should fill and submit Complex transaction form (1:2 split)', async ({
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

      // Fill the complex transaction form (1:2 transfer with split amounts)
      await fillComplexTransaction(
        page,
        COMPLEX_TRANSACTION_FORM_DATA.E2E_ONE_TO_TWO_TRANSFER
      )

      // Click the Review button
      await page.getByTestId('transaction-review-button').click()

      // Verify we're on the review step
      await expect(page.getByTestId('transaction-review-title')).toBeVisible({
        timeout: 10000
      })

      // Submit the transaction
      await page.getByTestId('transaction-submit-button').click()

      // Wait for redirect to transaction detail page
      await page.waitForURL(/\/transactions\/[a-f0-9-]{36}/, { timeout: 15000 })

      // Verify we're on the transaction detail page
      await expect(page).toHaveURL(/\/transactions\/[a-f0-9-]{36}/)
    })

    test('should fill and submit Complex transaction form (2:1 merge)', async ({
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

      // Fill the complex transaction form (2:1 transfer - merge from two sources)
      await fillComplexTransaction(
        page,
        COMPLEX_TRANSACTION_FORM_DATA.E2E_TWO_TO_ONE_TRANSFER
      )

      // Click the Review button
      await page.getByTestId('transaction-review-button').click()

      // Verify we're on the review step
      await expect(page.getByTestId('transaction-review-title')).toBeVisible({
        timeout: 10000
      })

      // Submit the transaction
      await page.getByTestId('transaction-submit-button').click()

      // Wait for redirect to transaction detail page
      await page.waitForURL(/\/transactions\/[a-f0-9-]{36}/, { timeout: 15000 })

      // Verify we're on the transaction detail page
      await expect(page).toHaveURL(/\/transactions\/[a-f0-9-]{36}/)
    })

    test('should fill and submit Complex transaction form (2:2 multi-split)', async ({
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

      // Fill the complex transaction form (2:2 transfer - multi-split)
      await fillComplexTransaction(
        page,
        COMPLEX_TRANSACTION_FORM_DATA.E2E_TWO_TO_TWO_TRANSFER
      )

      // Click the Review button
      await page.getByTestId('transaction-review-button').click()

      // Verify we're on the review step
      await expect(page.getByTestId('transaction-review-title')).toBeVisible({
        timeout: 10000
      })

      // Submit the transaction
      await page.getByTestId('transaction-submit-button').click()

      // Wait for redirect to transaction detail page
      await page.waitForURL(/\/transactions\/[a-f0-9-]{36}/, { timeout: 15000 })

      // Verify we're on the transaction detail page
      await expect(page).toHaveURL(/\/transactions\/[a-f0-9-]{36}/)
    })

    test('should show validation error for mismatched amounts (1:2 with invalid split)', async ({
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

      // Fill the complex transaction form with invalid amounts (50 + 30 = 80, but total is 100)
      await fillComplexTransaction(
        page,
        COMPLEX_TRANSACTION_FORM_DATA.E2E_ONE_TO_TWO_INVALID_AMOUNTS
      )

      // Click the Review button
      await page.getByTestId('transaction-review-button').click()

      // Wait a moment for validation to occur
      await page.waitForTimeout(1000)

      // Verify we're still on the form (not navigated to review)
      await expect(page.getByTestId('transaction-form-title')).toBeVisible()

      // Verify error message is displayed
      // The error should indicate that the amounts don't match the total
      const errorMessage = page.getByText(
        /total.*100|amount.*mismatch|distribute/i
      )
      await expect(errorMessage).toBeVisible({ timeout: 5000 })
    })

    test('should prevent adding the same account twice (duplicate handling)', async ({
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

      // Try to fill the complex transaction form with duplicate source account
      // The application should handle this seamlessly by preventing duplicates
      await fillComplexTransaction(
        page,
        COMPLEX_TRANSACTION_FORM_DATA.E2E_DUPLICATE_SOURCE_ACCOUNT
      )

      // Wait for form to settle
      await page.waitForTimeout(1000)

      // Verify that only 1 source account card is shown (duplicate was prevented)
      const accountCards = page.getByTestId('account-balance-card')
      const cardCount = await accountCards.count()

      // Should have exactly 2 cards: 1 source + 1 destination (not 3 or more)
      expect(cardCount).toBe(2)

      // The form should still be valid and allow proceeding to review
      await page.getByTestId('transaction-review-button').click()

      // Should successfully navigate to review
      await expect(page.getByTestId('transaction-review-title')).toBeVisible({
        timeout: 10000
      })
    })

    test('should show modal when source account has insufficient funds', async ({
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

      // Fill the complex transaction form with amount exceeding available balance
      // Account 5 has 100 BRL, but we're trying to transfer 900 BRL
      await fillComplexTransaction(
        page,
        COMPLEX_TRANSACTION_FORM_DATA.E2E_INSUFFICIENT_FUNDS
      )

      // Click the Review button
      await page.getByTestId('transaction-review-button').click()

      // Wait for insufficient balance modal to appear
      const modal = page.getByTestId('insufficient-balance-modal')
      await expect(modal).toBeVisible({ timeout: 5000 })

      // Verify modal title is displayed
      await expect(page.getByText(/accounts without balance/i)).toBeVisible()
    })

    test('should succeed when balance is updated on backend during frontend operation', async ({
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

      // Fill the complex transaction form with amount exceeding available balance
      // Account 5 has 100 BRL, but we're trying to transfer 900 BRL
      await fillComplexTransaction(
        page,
        COMPLEX_TRANSACTION_FORM_DATA.E2E_INSUFFICIENT_FUNDS
      )

      // Click the Review button
      await page.getByTestId('transaction-review-button').click()

      // Wait for insufficient balance modal to appear
      const modal = page.getByTestId('insufficient-balance-modal')
      await expect(modal).toBeVisible({ timeout: 5000 })

      // While modal is open, create a transaction on the backend to fund the account
      // This simulates the balance being updated during the frontend operation
      await createTransaction({
        chartOfAccountsGroupName: 'FUNDING',
        description: 'Backend deposit to fix balance during frontend operation',
        send: {
          asset: ASSETS.BRL.code,
          value: '900',
          source: {
            from: [
              {
                accountAlias: `@external/${ASSETS.BRL.code}`,
                amount: {
                  value: '900',
                  asset: ASSETS.BRL.code
                },
                chartOfAccounts: 'FUNDING_DEBIT',
                description: 'External BRL source'
              }
            ]
          },
          distribute: {
            to: [
              {
                accountAlias: ACCOUNTS.BRL_ACCOUNT_5.alias,
                amount: {
                  value: '900',
                  asset: ASSETS.BRL.code
                },
                chartOfAccounts: 'FUNDING_CREDIT',
                description: 'Credit to BRL account 5'
              }
            ]
          }
        }
      })

      // Now the account has enough funds on the backend (100 + 900 = 1000 BRL)
      // but the frontend still shows the old balance
      // Click "Continue Anyway" button to dismiss modal and proceed to review
      await page.getByTestId('insufficient-balance-continue-button').click()

      // Wait for review page to be visible
      await expect(page.getByTestId('transaction-review-title')).toBeVisible({
        timeout: 10000
      })

      // Submit the transaction
      await page.getByTestId('transaction-submit-button').click()

      // Wait for redirect to transaction detail page - should succeed!
      await page.waitForURL(/\/transactions\/[a-f0-9-]{36}/, { timeout: 15000 })

      // Verify we're on the transaction detail page
      await expect(page).toHaveURL(/\/transactions\/[a-f0-9-]{36}/)
    })

    test('should show warning modal when switching from complex to simple with data loss', async ({
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

      // Fill basic transaction info in simple mode
      await fillSimpleTransaction(
        page,
        SIMPLE_TRANSACTION_FORM_DATA.E2E_BRL_DEPOSIT
      )

      // Switch to complex mode
      await page.getByTestId('transaction-mode-change-button').click()
      await page.waitForTimeout(500)

      // Select complex mode from modal
      await page.getByTestId('mode-select-complex').click()
      await page.waitForTimeout(1000)

      // Now in complex mode - the form already has the filled data from simple mode
      // Add a second destination account to make it 1:2
      await page
        .getByTestId('complex-destination-account-search')
        .waitFor({ state: 'visible', timeout: 5000 })

      const destinationInput = page.getByTestId(
        'complex-destination-account-search'
      )
      await destinationInput.click()
      await page.waitForTimeout(300)

      await destinationInput.fill(ACCOUNTS.BRL_ACCOUNT_3.alias)
      await page.waitForTimeout(1500) // Wait for debounce and API response

      // Wait for and click the cmdk item
      const destinationOption = page
        .locator('[cmdk-item]')
        .filter({ hasText: ACCOUNTS.BRL_ACCOUNT_3.alias })
        .first()
      await destinationOption.waitFor({ state: 'visible', timeout: 10000 })
      await destinationOption.click()
      await page.waitForTimeout(300)

      // Click add button for second destination
      const addDestinationButton = page.getByTestId(
        'complex-destination-account-search-add-button'
      )
      await addDestinationButton.waitFor({ state: 'visible', timeout: 5000 })
      await addDestinationButton.click()
      await page.waitForTimeout(500)

      // Now we have 1:2 complex transaction
      // Verify we have 3 account cards (1 source + 2 destinations)
      const accountCards = page.getByTestId('account-balance-card')
      const cardCount = await accountCards.count()
      expect(cardCount).toBe(3)

      // Try to switch back to simple mode - should show warning
      const changeButton = page.getByTestId('transaction-mode-change-button')
      await changeButton.waitFor({ state: 'visible', timeout: 5000 })
      await changeButton.click()
      await page.waitForTimeout(1000)

      // Modal should appear with warning icon
      const modal = page.getByTestId('transaction-mode-modal')
      await expect(modal).toBeVisible({ timeout: 10000 })

      // Verify warning is visible (check for the triangle alert icon)
      const warningIcon = page.locator('[class*="text-red-600"]')
      await expect(warningIcon).toBeVisible()

      // Click simple mode to continue despite warning
      await page.getByTestId('mode-select-simple').click()
      await page.waitForTimeout(1000)

      // Verify we're back in simple mode with only 1 destination
      // Should have 2 account cards (1 source + 1 destination)
      const finalCardCount = await page
        .getByTestId('account-balance-card')
        .count()
      expect(finalCardCount).toBe(2)
    })

    test('should snap transaction value back to total when deleting second destination and switching to simple', async ({
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

      // Fill the complex transaction form (1:2 transfer with split amounts: 50 + 50 = 100)
      await fillComplexTransaction(
        page,
        COMPLEX_TRANSACTION_FORM_DATA.E2E_ONE_TO_TWO_TRANSFER
      )

      // Verify we have 3 account cards (1 source + 2 destinations)
      let accountCards = page.getByTestId('account-balance-card')
      let cardCount = await accountCards.count()
      expect(cardCount).toBe(3)

      // Delete the second destination account
      // The account cards have delete buttons - find and click the last destination's delete button
      const allCards = await page.getByTestId('account-balance-card').all()
      const lastCard = allCards[allCards.length - 1]

      // Find delete button within the last card (should be a trash icon or delete button)
      const deleteButton = lastCard.locator('button').first()
      await deleteButton.click()
      await page.waitForTimeout(500)

      // Verify we now have 2 account cards (1 source + 1 destination)
      accountCards = page.getByTestId('account-balance-card')
      cardCount = await accountCards.count()
      expect(cardCount).toBe(2)

      // Switch to simple mode
      const changeButton = page.getByTestId('transaction-mode-change-button')
      await changeButton.waitFor({ state: 'visible', timeout: 5000 })
      await changeButton.click()
      await page.waitForTimeout(500)

      // Select simple mode from modal (should not show warning since we now have 1:1)
      const modal = page.getByTestId('transaction-mode-modal')
      await expect(modal).toBeVisible({ timeout: 5000 })
      await page.getByTestId('mode-select-simple').click()
      await page.waitForTimeout(1000)

      // The destination operation should show 100 (full amount, not the split 50)
      // There should be 2 operation value displays: 1 debit (source) and 1 credit (destination)
      const operationValues = page.getByTestId('operation-value-display')
      const destinationValue = operationValues.last() // Last one is the credit/destination

      // Check that the destination shows 100 (snapped back from 50 to full transaction amount)
      await expect(destinationValue).toHaveText('100')
    })

    test('should show balance when clicking show balance on source account', async ({
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

      // Fill the simple transaction form
      await fillSimpleTransaction(
        page,
        SIMPLE_TRANSACTION_FORM_DATA.E2E_BRL_DEPOSIT
      )

      // Wait for source account card to be visible
      const accountCards = page.getByTestId('account-balance-card')
      await expect(accountCards.first()).toBeVisible()

      // Click "Show Balance" toggle on the source account (first card)
      const sourceCard = accountCards.first()
      const showBalanceButton = sourceCard.getByTestId('account-balance-toggle')
      await showBalanceButton.click()
      await page.waitForTimeout(1000)

      // Verify balance information is now visible
      // The balance section should be expanded showing balance details
      // Look for the collapsible content that contains the balance information
      const balanceInfo = sourceCard
        .locator('p')
        .filter({ hasText: /^BRL$/ })
        .first()
      await expect(balanceInfo).toBeVisible()

      // Verify balance section is expanded by checking for "Updated" text
      // The refresh button only appears if balance is older than 1 minute
      // Otherwise it shows "Updated" text with a checkmark
      const updatedText = sourceCard.getByText(/updated/i)
      await expect(updatedText).toBeVisible()
    })
  })
})
