import { test } from '@playwright/test'
import { navigateToLedgers } from '../utils/navigate-to-ledgers'

test('should navigate to Ledgers route via sidebar', async ({ page }) => {
  await navigateToLedgers(page)
})
