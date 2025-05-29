import { test, expect } from '@playwright/test'

import { navigateToLedgers } from '../utils/navigate-to-ledgers'

test.beforeEach(async ({ page }) => {
  await navigateToLedgers(page)
})

test('should create and delete a ledger', async ({ page }) => {
  await test.step('Create a new ledger', async () => {
    await page.getByTestId('new-ledger').click()
    await page.locator('input[name="name"]').fill('Test Ledger')
    await page.locator('#metadata').click()
    await page.locator('#key').fill('sample')
    await page.locator('#value').fill('metadata')
    await page.getByRole('button').first().click()
    await page.getByRole('button', { name: 'Save' }).click()
    await expect(page.getByTestId('ledgers-sheet')).not.toBeVisible()
    await expect(page.getByTestId('success-toast')).toBeVisible()
    await page.getByTestId('dismiss-toast').click()
  })

  await test.step('Delete the ledger', async () => {
    const testLedgerRow = page.getByRole('row', { name: 'Test Ledger' })
    await page.waitForLoadState('networkidle')
    await testLedgerRow.getByTestId('actions').click()
    await page.getByTestId('delete').click()
    await page.getByTestId('confirm').click()
    await expect(page.getByTestId('dialog')).not.toBeVisible()
    await expect(page.getByTestId('success-toast')).toBeVisible()
  })
})
