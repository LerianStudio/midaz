import { test, expect } from '@playwright/test'

import { navigateToLedgers } from '../utils/navigate-to-ledgers'

test.beforeEach(async ({ page }) => {
  await navigateToLedgers(page)
})

test('should create and delete a ledger', async ({ page }) => {
  const ledgerName = `E2E-Ledger-${Date.now()}`

  await test.step('Create a new ledger', async () => {
    // Use role-based selector - data-testid not rendered in DOM
    await page.getByRole('button', { name: /New Ledger|Novo Ledger/i }).click()

    // Wait for sheet to open by checking for the visible heading
    await expect(
      page.getByRole('heading', { name: /New Ledger|Novo Ledger/i })
    ).toBeVisible({ timeout: 15000 })

    // Fill in ledger name
    await page.locator('input[name="name"]').fill(ledgerName)

    // Save the ledger - wait for button to be enabled and visible
    const saveButton = page.getByRole('button', { name: /^Save$|^Salvar$/i })
    await expect(saveButton).toBeVisible()
    await expect(saveButton).toBeEnabled()
    await saveButton.click()

    // Wait for save operation to complete
    await page.waitForLoadState('networkidle')

    // Verify ledger appears in the list
    await expect(
      page.getByRole('row', { name: new RegExp(ledgerName) })
    ).toBeVisible({ timeout: 15000 })
  })

  await test.step('Delete the ledger', async () => {
    // Locate the ledger row
    const testLedgerRow = page.getByRole('row', {
      name: new RegExp(ledgerName)
    })

    // Wait for stable state
    await page.waitForLoadState('networkidle')

    // Open actions dropdown - use the last button in the row (three-dot menu)
    await testLedgerRow.getByRole('button').last().click()

    // Select delete option - use menu item role
    await page.getByRole('menuitem', { name: /Delete|Deletar/i }).click()

    // Confirm deletion - use button with "Confirm" text
    await page.getByRole('button', { name: /Confirm|Confirmar/i }).click()

    // Wait for deletion to complete
    await page.waitForLoadState('networkidle')

    // Verify success notification (use first() to avoid strict mode violation)
    await expect(
      page.getByText(/excluído com sucesso|successfully deleted/i).first()
    ).toBeVisible({ timeout: 10000 })
  })
})
