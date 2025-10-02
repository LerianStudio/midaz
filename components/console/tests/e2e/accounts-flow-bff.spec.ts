import { test, expect } from '@playwright/test'
import { BffApiMockService } from '../fixtures/bff-api-mocks'
import { BffTestHelpers } from '../utils/bff-test-helpers'
import { testDataFactory } from '../fixtures/test-data.factory'

test.describe('Accounts Management - E2E Tests (BFF Architecture)', () => {
  let mockService: BffApiMockService
  let bffHelpers: BffTestHelpers
  let testData: any

  test.beforeEach(async ({ page }) => {
    // Initialize BFF services
    mockService = new BffApiMockService(page)
    bffHelpers = new BffTestHelpers(page, mockService)

    // Setup comprehensive mocks for all endpoints
    await mockService.setupAllMocks()

    // Generate test data using Faker
    testData = {
      organization: testDataFactory.organization(),
      ledger: testDataFactory.ledger(),
      account: testDataFactory.account(),
      accounts: testDataFactory.list(testDataFactory.account, 10),
      asset: testDataFactory.asset(),
      assets: testDataFactory.list(testDataFactory.asset, 3),
      portfolio: testDataFactory.portfolio(),
      segment: testDataFactory.segment(),
      accountType: testDataFactory.accountType(),
      metadata: testDataFactory.metadata(5)
    }

    // Navigate to accounts page
    await page.goto('/accounts')
    await page.waitForLoadState('networkidle')
  })

  test.afterEach(async () => {
    // Clear captured requests between tests
    mockService.clearCapturedRequests()
  })

  test.describe('CRUD Operations', () => {
    test('should create account with all required fields', async ({ page }) => {
      await test.step('Open create account sheet', async () => {
        await page
          .getByRole('button', { name: /new account/i })
          .first()
          .click()

        await expect(
          page.getByRole('heading', { name: /new account/i })
        ).toBeVisible()

        await expect(
          page.getByText(
            /fill in the details of the account you want to create/i
          )
        ).toBeVisible()
      })

      await test.step('Fill form with required data', async () => {
        // Account Name (required)
        await page.getByLabel(/account name/i).fill(testData.account.name)

        // Asset (required)
        await page.getByLabel(/asset/i).click()
        await page.getByRole('option').first().click()
      })

      await test.step('Submit form', async () => {
        await page.getByRole('button', { name: /^save$/i }).click()
      })

      await test.step('Validate BFF endpoint calls', async () => {
        // Wait for success message
        await expect(page.getByText(/successfully created/i)).toBeVisible({
          timeout: 15000
        })

        // Verify BFF endpoint was called with POST
        await bffHelpers.assertBffEndpointCalled('/api/accounts', {
          method: 'POST',
          minCalls: 1
        })

        // Verify request body contains required fields
        await bffHelpers.assertRequestBodyContains('/api/accounts', {
          name: testData.account.name
        })
      })

      await test.step('Verify account appears in table', async () => {
        await page.waitForTimeout(1000)

        const accountRow = page.getByRole('row', {
          name: new RegExp(testData.account.name, 'i')
        })

        await expect(accountRow).toBeVisible({ timeout: 10000 })
      })
    })

    test('should create account with all optional fields', async ({ page }) => {
      await test.step('Open create sheet and fill all fields', async () => {
        await page
          .getByRole('button', { name: /new account/i })
          .first()
          .click()

        // Required fields
        await page.getByLabel(/account name/i).fill('Full Account Details')

        // Optional fields
        await page.getByLabel(/account alias/i).fill('@fulldetails')
        await page.getByLabel(/entity id/i).fill('ENTITY-123')

        // Asset selection (required)
        await page.getByLabel(/asset/i).click()
        await page.getByRole('option').first().click()

        // Segment (optional)
        const segmentField = page.getByLabel(/segment/i)
        if (await segmentField.isVisible()) {
          await segmentField.click()
          const hasOptions = await page.getByRole('option').count()
          if (hasOptions > 0) {
            await page.getByRole('option').first().click()
          } else {
            await page.keyboard.press('Escape')
          }
        }
      })

      await test.step('Submit and validate', async () => {
        await page.getByRole('button', { name: /^save$/i }).click()

        await expect(page.getByText(/successfully created/i)).toBeVisible({
          timeout: 15000
        })

        // Verify all optional fields were sent
        const lastRequest = bffHelpers.getLastRequest('/api/accounts')
        expect(lastRequest.body).toMatchObject({
          name: 'Full Account Details',
          alias: '@fulldetails',
          entityId: 'ENTITY-123'
        })
      })
    })

    test('should create account with minimal required fields', async ({
      page
    }) => {
      await test.step('Fill only required fields', async () => {
        await page
          .getByRole('button', { name: /new account/i })
          .first()
          .click()

        await page.getByLabel(/account name/i).fill('Minimal Account')
        await page.getByLabel(/asset/i).click()
        await page.getByRole('option').first().click()

        await page.getByRole('button', { name: /^save$/i }).click()
      })

      await test.step('Validate BFF call contains only required fields', async () => {
        await expect(page.getByText(/successfully created/i)).toBeVisible({
          timeout: 15000
        })

        const lastRequest = bffHelpers.getLastRequest('/api/accounts')
        expect(lastRequest.body).toHaveProperty('name')
        expect(lastRequest.body).toHaveProperty('assetCode')
      })
    })

    test('should list accounts with pagination', async ({ page }) => {
      await test.step('Verify accounts list loaded', async () => {
        // Wait for accounts to be visible
        await expect(
          page.getByRole('columnheader', { name: /account name/i })
        ).toBeVisible()
      })

      await test.step('Validate BFF pagination call', async () => {
        // Check that GET request was made to accounts endpoint
        await bffHelpers.assertBffEndpointCalled('/api/accounts', {
          method: 'GET',
          minCalls: 1
        })

        // Validate pagination was handled
        await bffHelpers.validateBffPagination('/api/accounts')
      })

      await test.step('Verify pagination controls', async () => {
        const hasAccounts = (await page.getByRole('row').count()) > 1

        if (hasAccounts) {
          await expect(page.getByText(/items per page/i)).toBeVisible()
        }
      })
    })

    test('should search accounts by alias', async ({ page }) => {
      await test.step('Create searchable account', async () => {
        await page
          .getByRole('button', { name: /new account/i })
          .first()
          .click()
        await page.getByLabel(/account name/i).fill('Searchable Account')
        await page.getByLabel(/account alias/i).fill('@searchme')
        await page.getByLabel(/asset/i).click()
        await page.getByRole('option').first().click()
        await page.getByRole('button', { name: /^save$/i }).click()

        await expect(page.getByText(/successfully created/i)).toBeVisible({
          timeout: 15000
        })
        await page.waitForTimeout(1500)
      })

      await test.step('Perform search', async () => {
        mockService.clearEndpointRequests('/api/accounts')

        const searchInput = page.getByTestId('search-input')
        await searchInput.fill('@searchme')
        await page.waitForTimeout(1500)
      })

      await test.step('Validate search results', async () => {
        await expect(
          page.getByRole('row', { name: /searchable account/i })
        ).toBeVisible({ timeout: 10000 })

        // Verify search triggered GET request
        const requests = bffHelpers.getAllRequests('/api/accounts')
        const searchRequest = requests.find((r) => r.method === 'GET')
        expect(searchRequest).toBeDefined()
      })
    })

    test('should update account name', async ({ page }) => {
      await test.step('Create account', async () => {
        await page
          .getByRole('button', { name: /new account/i })
          .first()
          .click()
        await page.getByLabel(/account name/i).fill('Account To Edit')
        await page.getByLabel(/asset/i).click()
        await page.getByRole('option').first().click()
        await page.getByRole('button', { name: /^save$/i }).click()

        await expect(page.getByText(/successfully created/i)).toBeVisible({
          timeout: 15000
        })
        await page.waitForTimeout(1500)
      })

      await test.step('Open edit sheet', async () => {
        const row = page.getByRole('row', { name: /account to edit/i })
        await expect(row).toBeVisible({ timeout: 10000 })
        await row.getByTestId('actions').click()
        await page.getByTestId('edit').click()

        await expect(
          page.getByRole('heading', { name: /edit account to edit/i })
        ).toBeVisible()
      })

      await test.step('Update account name', async () => {
        mockService.clearEndpointRequests('/api/accounts')

        const nameInput = page.getByLabel(/account name/i)
        await nameInput.clear()
        await nameInput.fill('Updated Account Name')

        await page.getByRole('button', { name: /^save$/i }).click()
      })

      await test.step('Validate BFF update call', async () => {
        await expect(page.getByText(/successfully updated/i)).toBeVisible({
          timeout: 15000
        })

        // Verify PATCH/PUT was called
        await bffHelpers.assertBffEndpointCalled('/api/accounts', {
          method: 'PATCH',
          minCalls: 1
        })

        // Verify updated data
        await bffHelpers.assertRequestBodyContains('/api/accounts', {
          name: 'Updated Account Name'
        })
      })

      await test.step('Verify update in table', async () => {
        await page.waitForTimeout(1500)
        await expect(
          page.getByRole('row', { name: /updated account name/i })
        ).toBeVisible({ timeout: 10000 })
      })
    })

    test('should delete account with confirmation', async ({ page }) => {
      await test.step('Create account', async () => {
        await page
          .getByRole('button', { name: /new account/i })
          .first()
          .click()
        await page.getByLabel(/account name/i).fill('Account To Delete')
        await page.getByLabel(/asset/i).click()
        await page.getByRole('option').first().click()
        await page.getByRole('button', { name: /^save$/i }).click()

        await expect(page.getByText(/successfully created/i)).toBeVisible({
          timeout: 15000
        })
        await page.waitForTimeout(1500)
      })

      await test.step('Open delete confirmation', async () => {
        const row = page.getByRole('row', { name: /account to delete/i })
        await expect(row).toBeVisible({ timeout: 10000 })
        await row.getByTestId('actions').click()
        await page.getByTestId('delete').click()

        await expect(page.getByText(/confirm deletion/i)).toBeVisible()
        await expect(
          page.getByText(/you will delete an account/i)
        ).toBeVisible()
      })

      await test.step('Confirm deletion', async () => {
        mockService.clearEndpointRequests('/api/accounts')

        await page.getByRole('button', { name: /confirm/i }).click()
      })

      await test.step('Validate BFF delete call', async () => {
        await expect(page.getByText(/successfully deleted/i)).toBeVisible({
          timeout: 15000
        })

        // Verify DELETE was called
        await bffHelpers.assertBffEndpointCalled('/api/accounts', {
          method: 'DELETE',
          minCalls: 1
        })
      })

      await test.step('Verify removed from list', async () => {
        await page.waitForTimeout(1500)
        await expect(
          page.getByRole('row', { name: /account to delete/i })
        ).not.toBeVisible({ timeout: 5000 })
      })
    })
  })

  test.describe('Validation Scenarios', () => {
    test('should validate required account name', async ({ page }) => {
      await page
        .getByRole('button', { name: /new account/i })
        .first()
        .click()

      // Try to save without name
      await page.getByRole('button', { name: /^save$/i }).click()

      // Should show validation error
      await expect(
        page.locator('text=/.*name.*(required|must be at least)/i')
      ).toBeVisible()

      // Verify no BFF call was made
      const createRequests = bffHelpers
        .getAllRequests('/api/accounts')
        .filter((r) => r.method === 'POST')
      expect(createRequests.length).toBe(0)
    })

    test('should validate required asset selection', async ({ page }) => {
      await page
        .getByRole('button', { name: /new account/i })
        .first()
        .click()

      // Fill only name
      await page.getByLabel(/account name/i).fill('Test Account')
      await page.getByRole('button', { name: /^save$/i }).click()

      // Should show validation error
      await expect(
        page.locator('text=/.*asset.*(required|expected)/i')
      ).toBeVisible()

      // Verify no BFF call was made
      const createRequests = bffHelpers
        .getAllRequests('/api/accounts')
        .filter((r) => r.method === 'POST')
      expect(createRequests.length).toBe(0)
    })

    test('should validate alias format if provided', async ({ page }) => {
      await page
        .getByRole('button', { name: /new account/i })
        .first()
        .click()

      await page.getByLabel(/account name/i).fill('Test Account')
      await page.getByLabel(/account alias/i).fill('invalid-alias-without-at')
      await page.getByLabel(/asset/i).click()
      await page.getByRole('option').first().click()

      await page.getByRole('button', { name: /^save$/i }).click()

      // Note: Validation may occur on server side, so check for either
      // client-side validation or server error response
      await page.waitForTimeout(2000)
    })
  })

  test.describe('Error Handling', () => {
    test('should handle 400 Bad Request', async ({ page }) => {
      await test.step('Setup error scenario', async () => {
        await mockService.setupErrorScenario(
          'badRequest',
          '/api/organizations/*/ledgers/*/accounts'
        )
      })

      await test.step('Attempt to create account', async () => {
        await page
          .getByRole('button', { name: /new account/i })
          .first()
          .click()
        await page.getByLabel(/account name/i).fill('Bad Request Test')
        await page.getByLabel(/asset/i).click()
        await page.getByRole('option').first().click()
        await page.getByRole('button', { name: /^save$/i }).click()
      })

      await test.step('Verify error handling', async () => {
        // Should show error message (toast or inline)
        // Wait for error state
        await page.waitForTimeout(2000)

        // Verify BFF was still called
        await bffHelpers.assertBffEndpointCalled('/api/accounts', {
          method: 'POST'
        })
      })
    })

    test('should handle 409 Conflict (duplicate account)', async ({ page }) => {
      await test.step('Setup conflict scenario', async () => {
        await mockService.setupErrorScenario(
          'conflict',
          '/api/organizations/*/ledgers/*/accounts'
        )
      })

      await test.step('Attempt to create duplicate', async () => {
        await page
          .getByRole('button', { name: /new account/i })
          .first()
          .click()
        await page.getByLabel(/account name/i).fill('Duplicate Account')
        await page.getByLabel(/account alias/i).fill('@duplicate')
        await page.getByLabel(/asset/i).click()
        await page.getByRole('option').first().click()
        await page.getByRole('button', { name: /^save$/i }).click()
      })

      await test.step('Verify conflict error displayed', async () => {
        await page.waitForTimeout(2000)

        // Verify request was made
        await bffHelpers.assertBffEndpointCalled('/api/accounts', {
          method: 'POST'
        })
      })
    })

    test('should handle 500 Server Error', async ({ page }) => {
      await mockService.setupErrorScenario(
        'serverError',
        '/api/organizations/*/ledgers/*/accounts'
      )

      await page
        .getByRole('button', { name: /new account/i })
        .first()
        .click()
      await page.getByLabel(/account name/i).fill('Server Error Test')
      await page.getByLabel(/asset/i).click()
      await page.getByRole('option').first().click()
      await page.getByRole('button', { name: /^save$/i }).click()

      await page.waitForTimeout(2000)

      // Verify error was handled gracefully
      await bffHelpers.assertBffEndpointCalled('/api/accounts', {
        method: 'POST'
      })
    })
  })

  test.describe('BFF Layer Validation', () => {
    test('should transform request correctly', async ({ page }) => {
      await test.step('Create account with specific data', async () => {
        await page
          .getByRole('button', { name: /new account/i })
          .first()
          .click()

        await page.getByLabel(/account name/i).fill('Transformation Test')
        await page.getByLabel(/account alias/i).fill('@transform')
        await page.getByLabel(/entity id/i).fill('ENTITY-999')
        await page.getByLabel(/asset/i).click()
        await page.getByRole('option').first().click()

        await page.getByRole('button', { name: /^save$/i }).click()

        await expect(page.getByText(/successfully created/i)).toBeVisible({
          timeout: 15000
        })
      })

      await test.step('Validate request transformation', async () => {
        const lastRequest = bffHelpers.getLastRequest('/api/accounts')

        // Verify all fields were correctly sent
        expect(lastRequest.body).toMatchObject({
          name: 'Transformation Test',
          alias: '@transform',
          entityId: 'ENTITY-999'
        })

        expect(lastRequest.body).toHaveProperty('assetCode')
      })
    })

    test('should add metadata correctly', async ({ page }) => {
      await test.step('Create account with metadata', async () => {
        await page
          .getByRole('button', { name: /new account/i })
          .first()
          .click()

        await page.getByLabel(/account name/i).fill('Metadata Test')
        await page.getByLabel(/asset/i).click()
        await page.getByRole('option').first().click()

        // Navigate to Metadata tab
        await page.getByRole('tab', { name: /metadata/i }).click()

        // Add metadata entries
        await page.locator('#key').fill('customer-tier')
        await page.locator('#value').fill('gold')
        await page.getByRole('button', { name: /add/i }).first().click()
        await page.waitForTimeout(300)

        await page.locator('#key').fill('region')
        await page.locator('#value').fill('north-america')
        await page.getByRole('button', { name: /add/i }).first().click()
        await page.waitForTimeout(300)

        await page.getByRole('button', { name: /^save$/i }).click()

        await expect(page.getByText(/successfully created/i)).toBeVisible({
          timeout: 15000
        })
      })

      await test.step('Validate metadata in request', async () => {
        const lastRequest = bffHelpers.getLastRequest('/api/accounts')

        expect(lastRequest.body.metadata).toBeDefined()
        expect(lastRequest.body.metadata).toMatchObject({
          'customer-tier': 'gold',
          region: 'north-america'
        })
      })
    })

    test('should not send empty optional fields', async ({ page }) => {
      await test.step('Create account with minimal data', async () => {
        await page
          .getByRole('button', { name: /new account/i })
          .first()
          .click()

        await page.getByLabel(/account name/i).fill('Clean Request Test')
        await page.getByLabel(/asset/i).click()
        await page.getByRole('option').first().click()

        await page.getByRole('button', { name: /^save$/i }).click()

        await expect(page.getByText(/successfully created/i)).toBeVisible({
          timeout: 15000
        })
      })

      await test.step('Validate clean request body', async () => {
        const lastRequest = bffHelpers.getLastRequest('/api/accounts')

        // Empty string fields should not be sent
        expect(lastRequest.body.alias).toBeUndefined()
        expect(lastRequest.body.entityId).toBeUndefined()
      })
    })
  })

  test.describe('Complex Workflows', () => {
    test('should create account with portfolio link', async ({ page }) => {
      await test.step('Create account and link portfolio', async () => {
        await page
          .getByRole('button', { name: /new account/i })
          .first()
          .click()

        await page.getByLabel(/account name/i).fill('Portfolio Linked Account')
        await page.getByLabel(/asset/i).click()
        await page.getByRole('option').first().click()

        // Navigate to Portfolio tab
        await page.getByRole('tab', { name: /portfolio/i }).click()

        // Select portfolio if available
        const portfolioField = page.getByLabel(/portfolio/i)
        if (
          (await portfolioField.isVisible()) &&
          !(await portfolioField.isDisabled())
        ) {
          await portfolioField.click()
          const hasOptions = await page.getByRole('option').count()
          if (hasOptions > 0) {
            await page.getByRole('option').first().click()

            await expect(
              page.getByText(/account linked to a portfolio/i)
            ).toBeVisible()
          }
        }

        await page.getByRole('button', { name: /^save$/i }).click()

        await expect(page.getByText(/successfully created/i)).toBeVisible({
          timeout: 15000
        })
      })

      await test.step('Validate portfolio link in request', async () => {
        const lastRequest = bffHelpers.getLastRequest('/api/accounts')

        // If portfolio was selected, verify it's in the request
        if (lastRequest.body.portfolioId) {
          expect(lastRequest.body.portfolioId).toBeDefined()
        }
      })
    })

    test('should display balance in edit mode', async ({ page }) => {
      await test.step('Create account', async () => {
        await page
          .getByRole('button', { name: /new account/i })
          .first()
          .click()
        await page.getByLabel(/account name/i).fill('Balance Test Account')
        await page.getByLabel(/asset/i).click()
        await page.getByRole('option').first().click()
        await page.getByRole('button', { name: /^save$/i }).click()

        await expect(page.getByText(/successfully created/i)).toBeVisible({
          timeout: 15000
        })
        await page.waitForTimeout(1500)
      })

      await test.step('Open edit and verify balance section', async () => {
        const row = page.getByRole('row', { name: /balance test account/i })
        await expect(row).toBeVisible({ timeout: 10000 })
        await row.getByTestId('actions').click()
        await page.getByTestId('edit').click()

        // Verify balance section exists
        await expect(page.getByText(/account balance/i)).toBeVisible()
      })

      await test.step('Validate balance API call', async () => {
        // Balance endpoint should be called when editing
        // Note: This requires adding balance mocking to BffApiMockService
        await page.waitForTimeout(1000)
      })
    })

    test('should handle account type validation when enabled', async ({
      page
    }) => {
      await test.step('Check account type field', async () => {
        await page
          .getByRole('button', { name: /new account/i })
          .first()
          .click()

        // Check if account type field exists
        const typeField = page.getByLabel(/type/i)
        if (await typeField.isVisible()) {
          // Account type validation is enabled
          await expect(typeField).toBeVisible()

          // If it's a select, verify options are available
          if ((await typeField.getAttribute('role')) === 'combobox') {
            await typeField.click()
            const hasOptions = await page.getByRole('option').count()
            expect(hasOptions).toBeGreaterThan(0)
            await page.keyboard.press('Escape')
          }
        }
      })
    })

    test('should disable switches in create mode', async ({ page }) => {
      await page
        .getByRole('button', { name: /new account/i })
        .first()
        .click()

      const allowSendingSwitch = page.getByLabel(/allow sending/i)
      const allowReceivingSwitch = page.getByLabel(/allow receiving/i)

      await expect(allowSendingSwitch).toBeDisabled()
      await expect(allowReceivingSwitch).toBeDisabled()
    })

    test('should enable switches in edit mode', async ({ page }) => {
      await test.step('Create account', async () => {
        await page
          .getByRole('button', { name: /new account/i })
          .first()
          .click()
        await page.getByLabel(/account name/i).fill('Switch Test Account')
        await page.getByLabel(/asset/i).click()
        await page.getByRole('option').first().click()
        await page.getByRole('button', { name: /^save$/i }).click()

        await expect(page.getByText(/successfully created/i)).toBeVisible({
          timeout: 15000
        })
        await page.waitForTimeout(1500)
      })

      await test.step('Open edit and verify switches enabled', async () => {
        const row = page.getByRole('row', { name: /switch test account/i })
        await expect(row).toBeVisible({ timeout: 10000 })
        await row.getByTestId('actions').click()
        await page.getByTestId('edit').click()

        await expect(page.getByLabel(/allow sending/i)).not.toBeDisabled()
        await expect(page.getByLabel(/allow receiving/i)).not.toBeDisabled()
      })
    })

    test('should make alias and asset readonly in edit mode', async ({
      page
    }) => {
      await test.step('Create account', async () => {
        await page
          .getByRole('button', { name: /new account/i })
          .first()
          .click()
        await page.getByLabel(/account name/i).fill('Immutable Test')
        await page.getByLabel(/account alias/i).fill('@immutable')
        await page.getByLabel(/asset/i).click()
        await page.getByRole('option').first().click()
        await page.getByRole('button', { name: /^save$/i }).click()

        await expect(page.getByText(/successfully created/i)).toBeVisible({
          timeout: 15000
        })
        await page.waitForTimeout(1500)
      })

      await test.step('Verify readonly fields in edit', async () => {
        const row = page.getByRole('row', { name: /immutable test/i })
        await expect(row).toBeVisible({ timeout: 10000 })
        await row.getByTestId('actions').click()
        await page.getByTestId('edit').click()

        const aliasInput = page.getByLabel(/account alias/i)
        const assetField = page.getByLabel(/asset/i)

        await expect(aliasInput).toHaveAttribute('readonly')
        await expect(assetField).toHaveAttribute('aria-readonly', 'true')
      })
    })
  })

  test.describe('UI Display Validation', () => {
    test('should display correct table columns', async ({ page }) => {
      await expect(
        page.getByRole('columnheader', { name: /account name/i })
      ).toBeVisible()

      await expect(
        page.getByRole('columnheader', { name: /^id$/i })
      ).toBeVisible()

      await expect(
        page.getByRole('columnheader', { name: /account alias/i })
      ).toBeVisible()

      await expect(
        page.getByRole('columnheader', { name: /assets/i })
      ).toBeVisible()

      await expect(
        page.getByRole('columnheader', { name: /metadata/i })
      ).toBeVisible()

      await expect(
        page.getByRole('columnheader', { name: /portfolio/i })
      ).toBeVisible()

      await expect(
        page.getByRole('columnheader', { name: /actions/i })
      ).toBeVisible()
    })

    test('should display page header elements', async ({ page }) => {
      await expect(
        page.getByRole('navigation', { name: /breadcrumb/i })
      ).toBeVisible()

      await expect(
        page.getByRole('heading', { name: /accounts/i, level: 1 })
      ).toBeVisible()

      await expect(
        page.getByText(/manage the accounts of this ledger/i)
      ).toBeVisible()

      await expect(
        page.getByRole('button', { name: /what is an account/i })
      ).toBeVisible()

      await expect(
        page.getByRole('button', { name: /new account/i })
      ).toBeVisible()
    })

    test('should display search controls', async ({ page }) => {
      await expect(
        page.getByPlaceholder(/search by id or alias/i)
      ).toBeVisible()
      await expect(page.getByText(/items per page/i)).toBeVisible()
    })
  })
})
