import { Page, expect } from '@playwright/test'

export async function navigateToAssets(page: Page) {
  // CRITICAL: Assets page requires a ledger to be selected
  // First, ensure we're starting from a page that has ledger context
  await page.goto('/ledgers', { waitUntil: 'domcontentloaded' })

  // Wait for ledgers page to load - support both English and Portuguese
  await expect(
    page.getByRole('heading', { name: /^Ledgers$|^Ledgers$/i, level: 1 })
  ).toBeVisible({ timeout: 15000 })

  // Verify a ledger exists and is selected (look for "current" indicator)
  const currentLedgerExists = await page
    .getByText(/(atual|current)/i)
    .isVisible({ timeout: 5000 })
    .catch(() => false)

  if (!currentLedgerExists) {
    throw new Error(
      'No ledger is currently selected. Assets page requires an active ledger. ' +
        'Please ensure at least one ledger exists before testing assets.'
    )
  }

  // Now navigate to assets page
  await page.goto('/assets', { waitUntil: 'domcontentloaded' })

  // Check if error boundary is showing
  const errorHeading = page.getByRole('heading', {
    name: /application error/i
  })

  if (await errorHeading.isVisible({ timeout: 2000 }).catch(() => false)) {
    // Capture any console errors for debugging
    throw new Error(
      `Application crashed when loading /assets page. ` +
        `This likely means no ledger is selected or there's a data loading issue.`
    )
  }

  // Wait for the page heading to be visible - support both languages
  await expect(
    page.getByRole('heading', { name: /^Assets$|^Ativos$/i, level: 1 })
  ).toBeVisible({ timeout: 15000 })

  // Wait for the new asset button using role-based selector (data-testid not rendered)
  await expect(
    page.getByRole('button', { name: /New Asset|Novo Ativo/i })
  ).toBeAttached({ timeout: 10000 })
}
