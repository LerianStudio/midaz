/**
 * ORGANIZATIONS E2E TESTS - SIMPLE & WORKING
 *
 * Uses only selectors that actually exist on the rendered page
 * No data-testid required - works immediately
 */

import { test, expect } from '@playwright/test'
import { testDataFactory } from '../fixtures/test-data.factory'
import { navigateToOrganizations } from '../utils/navigate-to-organizations'

test.describe('Organizations - Basic Tests', () => {
  test.beforeEach(async ({ page }) => {
    await navigateToOrganizations(page)
    await page.waitForLoadState('networkidle')

    // Wait for page to be ready
    await page.waitForSelector(
      'table, button:has-text("Create"), button:has-text("Criar")',
      {
        timeout: 10000
      }
    )
  })

  test('should show organizations page', async ({ page }) => {
    // Verify we're on the right page by checking for key elements
    await expect(
      page.getByRole('button', { name: /Create|Criar/i })
    ).toBeVisible()
    await expect(
      page.getByPlaceholder(/Search by ID|Buscar por ID/i)
    ).toBeVisible()
    await expect(page.locator('table')).toBeVisible()
  })

  test('should display organizations table', async ({ page }) => {
    const table = page.locator('table')
    await expect(table).toBeVisible()

    // Verify table has rows
    const rows = page.locator('table tbody tr')
    const count = await rows.count()
    expect(count).toBeGreaterThan(0)
  })

  test('should navigate to create form', async ({ page }) => {
    // Click create button (works in both EN and PT)
    await page.getByRole('button', { name: /Create|Criar/i }).click()
    await page.waitForLoadState('networkidle')

    // Verify URL changed to new-organization
    await expect(page).toHaveURL(/new-organization/)

    // Verify form fields are present
    await expect(page.locator('input[name="legalName"]')).toBeVisible()
    await expect(page.locator('input[name="doingBusinessAs"]')).toBeVisible()
  })

  test('should create organization with form inputs', async ({ page }) => {
    const orgData = testDataFactory.organization()

    // Navigate to create form
    await page.getByRole('button', { name: /Create|Criar/i }).click()
    await page.waitForLoadState('networkidle')

    // Fill form using name attributes
    await page.locator('input[name="legalName"]').fill(orgData.legalName)
    await page.locator('input[name="doingBusinessAs"]').fill('E2E Test DBA')
    await page
      .locator('input[name="legalDocument"]')
      .fill(orgData.legalDocument)
    await page.locator('input[name="address.line1"]').fill('123 Test Street')
    await page.locator('input[name="address.city"]').fill('Test City')
    await page.locator('input[name="address.zipCode"]').fill('12345')

    // Select country - use nth to skip the disabled ledger selector at top
    const countrySelect = page.locator('button[role="combobox"]').nth(1)
    await countrySelect.click()
    await page.locator('[role="option"]').first().click()

    // Select state - use nth(2) for the state selector
    const stateSelect = page.locator('button[role="combobox"]').nth(2)
    await stateSelect.click()
    await page.locator('[role="option"]').first().click()

    // Submit form (Save button)
    await page.getByRole('button', { name: /Save|Salvar/i }).click()

    // Verify success toast appears (works in EN or PT) - use .first() to avoid strict mode
    await expect(
      page.locator('text=/Organization created|Organização criada/i').first()
    ).toBeVisible({ timeout: 10000 })

    // Verify redirect back to settings (tab might or might not be in URL)
    await expect(page).toHaveURL(/settings(\?|$)/, {
      timeout: 10000
    })

    // Reload and verify organization appears in table
    await page.reload()
    await page.waitForLoadState('networkidle')

    await expect(page.locator(`text="${orgData.legalName}"`)).toBeVisible({
      timeout: 10000
    })
  })

  test('should search by organization ID', async ({ page }) => {
    // Get first row ID
    const firstRow = page.locator('table tbody tr').first()
    await firstRow.waitFor({ state: 'visible' })

    // Second cell contains the ID
    const idCell = firstRow.locator('td').nth(1)
    const idText = await idCell.textContent()
    const orgId = idText?.trim() || ''

    if (!orgId) {
      console.log('No organization ID found, skipping search test')
      return
    }

    // Search by ID
    const searchInput = page.getByPlaceholder(/Search by ID|Buscar por ID/i)
    await searchInput.fill(orgId)
    await searchInput.press('Enter')
    await page.waitForLoadState('networkidle')

    // Verify organization still visible
    await expect(page.locator(`text="${orgId}"`)).toBeVisible()
  })

  test('should open actions menu', async ({ page }) => {
    // Find first row's action button (last cell)
    const firstRow = page.locator('table tbody tr').first()
    const actionButton = firstRow.locator('td').last().locator('button')

    await actionButton.click()

    // Verify menu appears (check for Edit option in EN or PT)
    await expect(
      page
        .locator('[role="menuitem"], [role="option"]')
        .filter({ hasText: /Edit|Editar/i })
    ).toBeVisible({ timeout: 5000 })
  })
})

test.describe('Organizations - Full CRUD', () => {
  test('should create and delete organization', async ({ page }) => {
    await navigateToOrganizations(page)
    await page.waitForLoadState('networkidle')

    const orgData = testDataFactory.organization()
    const uniqueName = `${orgData.legalName} ${Date.now()}`

    // ========== CREATE ==========
    await page.getByRole('button', { name: /Create|Criar/i }).click()
    await page.waitForLoadState('networkidle')

    // Fill form
    await page.locator('input[name="legalName"]').fill(uniqueName)
    await page.locator('input[name="doingBusinessAs"]').fill('CRUD Test')
    await page
      .locator('input[name="legalDocument"]')
      .fill(orgData.legalDocument)
    await page.locator('input[name="address.line1"]').fill('456 Street')
    await page.locator('input[name="address.city"]').fill('City')
    await page.locator('input[name="address.zipCode"]').fill('99999')

    // Select dropdowns (nth(1) = country, nth(2) = state, nth(0) = ledger which is disabled)
    await page.locator('button[role="combobox"]').nth(1).click()
    await page.locator('[role="option"]').first().click()

    await page.locator('button[role="combobox"]').nth(2).click()
    await page.locator('[role="option"]').first().click()

    // Save
    await page.getByRole('button', { name: /Save|Salvar/i }).click()

    // Wait for success
    await expect(
      page.locator('text=/Organization created|Organização criada/i').first()
    ).toBeVisible({ timeout: 10000 })

    // Verify redirect back to settings (tab might or might not be in URL)
    await expect(page).toHaveURL(/settings(\?|$)/, {
      timeout: 10000
    })

    // Reload to see new organization
    await page.reload()
    await page.waitForLoadState('networkidle')

    // ========== VERIFY EXISTS ==========
    const orgRow = page
      .locator('table tbody tr')
      .filter({ hasText: uniqueName })
    await expect(orgRow).toBeVisible({ timeout: 10000 })

    // ========== DELETE ==========
    const actionButton = orgRow.locator('td').last().locator('button')
    await actionButton.click()

    // Click delete option
    await page
      .locator('[role="menuitem"], [role="option"]')
      .filter({
        hasText: /Delete|Excluir|Deletar/i
      })
      .click()

    // Confirm in dialog
    await expect(page.getByRole('dialog')).toBeVisible({ timeout: 5000 })

    // Click confirm button
    await page
      .getByRole('button', { name: /Confirm|Delete|Confirmar|Excluir/i })
      .click()

    // Verify success
    await expect(
      page.locator('text=/deleted|excluíd|deletad/i').first()
    ).toBeVisible({
      timeout: 10000
    })

    // Verify row disappears
    await expect(orgRow).not.toBeVisible({ timeout: 10000 })
  })
})
