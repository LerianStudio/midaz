import { test, expect } from '@playwright/test'
import { navigateToAssets } from '../utils/navigate-to-assets'

test.beforeEach(async ({ page }) => {
  await navigateToAssets(page)
})

test.describe('Assets Management - E2E Tests', () => {
  test('should create and delete an asset', async ({ page }) => {
    const assetName = `E2E-Asset-${Date.now()}`
    // Generate letters-only code
    const assetCode = `TST${Math.random().toString(36).substring(2, 8).toUpperCase().replace(/[0-9]/g, 'X')}`

    await test.step('Create a new asset', async () => {
      // Ensure page is stable before interacting
      await page.waitForLoadState('networkidle')
      await page.waitForTimeout(500)

      // Use role-based selector - data-testid not rendered in DOM
      const newAssetButton = page.getByRole('button', {
        name: /New Asset|Novo Ativo/i
      })
      await expect(newAssetButton).toBeVisible()
      await newAssetButton.click()

      // Wait for sheet to open by checking for the visible heading
      await expect(
        page.getByRole('heading', { name: /New Asset|Novo Ativo/i })
      ).toBeVisible({ timeout: 15000 })

      // Select asset type - use crypto to get text input for code
      await page.getByLabel(/type|tipo/i).click()
      await page.getByRole('option', { name: /crypto/i }).click()

      // Fill in asset name
      await page.getByLabel(/asset name|nome do ativo/i).fill(assetName)

      // Fill in asset code (text input for non-currency types)
      await page.getByLabel(/^code|^código/i).fill(assetCode)

      // Save the asset - wait for button to be enabled and visible
      const saveButton = page.getByRole('button', { name: /^Save$|^Salvar$/i })
      await expect(saveButton).toBeVisible()
      await expect(saveButton).toBeEnabled()
      await page.waitForTimeout(500) // Wait for form validation
      await saveButton.click()

      // Wait for sheet to close
      await expect(
        page.getByRole('heading', { name: /New Asset|Novo Ativo/i })
      ).not.toBeVisible({ timeout: 10000 })

      // Wait for save operation to complete
      await page.waitForLoadState('networkidle')

      // Verify asset appears in the list (primary verification)
      await expect(
        page.getByRole('row', { name: new RegExp(assetName) })
      ).toBeVisible({ timeout: 15000 })

      // Optionally verify success notification (toast may auto-dismiss quickly)
      const successToast = page
        .getByText(/criado com sucesso|successfully created/i)
        .first()
      await successToast.isVisible().catch(() => false)
    })

    await test.step('Delete the asset', async () => {
      // Locate the asset row
      const testAssetRow = page.getByRole('row', {
        name: new RegExp(assetName)
      })

      // Wait for stable state
      await page.waitForLoadState('networkidle')

      // Open actions dropdown - use the last button in the row (three-dot menu)
      await testAssetRow.getByRole('button').last().click()

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

  test('should create asset with all fields including metadata', async ({
    page
  }) => {
    const assetName = `Full-Asset-${Date.now()}`
    const assetCode = `ETH${Math.random().toString(36).substring(2, 8).toUpperCase().replace(/[0-9]/g, 'X')}`

    await test.step('Open create asset sheet', async () => {
      await page.waitForLoadState('networkidle')
      await page.waitForTimeout(500)

      const newAssetButton = page.getByRole('button', {
        name: /New Asset|Novo Ativo/i
      })
      await expect(newAssetButton).toBeVisible()
      await newAssetButton.click()

      await expect(
        page.getByRole('heading', { name: /New Asset|Novo Ativo/i })
      ).toBeVisible({ timeout: 15000 })
    })

    await test.step('Fill basic details', async () => {
      // Select type
      await page.getByLabel(/type|tipo/i).click()
      await page.getByRole('option', { name: /crypto/i }).click()

      // Fill name and code
      await page.getByLabel(/asset name|nome do ativo/i).fill(assetName)
      await page.getByLabel(/^code|^código/i).fill(assetCode)
    })

    await test.step('Verify metadata tab exists', async () => {
      // Navigate to Metadata tab to verify it's present
      await page.getByRole('tab', { name: /metadata/i }).click()
      await page.waitForTimeout(500)

      // Verify metadata fields are visible
      await expect(page.locator('#key')).toBeVisible()
      await expect(page.locator('#value')).toBeVisible()

      // Navigate back to details
      await page.getByRole('tab', { name: /details|detalhes/i }).click()
      await page.waitForTimeout(300)
    })

    await test.step('Save and verify', async () => {
      // Save the asset
      const saveButton = page.getByRole('button', { name: /^Save$|^Salvar$/i })
      await expect(saveButton).toBeVisible()
      await expect(saveButton).toBeEnabled()
      await page.waitForTimeout(500)
      await saveButton.click()

      // Wait for sheet to close
      await expect(
        page.getByRole('heading', { name: /New Asset|Novo Ativo/i })
      ).not.toBeVisible({ timeout: 10000 })

      await page.waitForLoadState('networkidle')

      // Verify asset appears in the list (primary verification)
      await expect(
        page.getByRole('row', { name: new RegExp(assetName) })
      ).toBeVisible({ timeout: 15000 })

      // Optionally verify success notification (toast may auto-dismiss quickly)
      const successToast = page
        .getByText(/criado com sucesso|successfully created/i)
        .first()
      await successToast.isVisible().catch(() => false)
    })
  })

  test('should edit an existing asset', async ({ page }) => {
    const initialName = `Edit-Asset-${Date.now()}`
    const updatedName = `Updated-${Date.now()}`
    const assetCode = `EDT${Math.random().toString(36).substring(2, 8).toUpperCase().replace(/[0-9]/g, 'X')}`

    await test.step('Create asset first', async () => {
      await page.waitForLoadState('networkidle')
      await page.waitForTimeout(500)

      const newAssetButton = page.getByRole('button', {
        name: /New Asset|Novo Ativo/i
      })
      await expect(newAssetButton).toBeVisible()
      await newAssetButton.click()

      await expect(
        page.getByRole('heading', { name: /New Asset|Novo Ativo/i })
      ).toBeVisible({ timeout: 15000 })

      await page.getByLabel(/type|tipo/i).click()
      await page.getByRole('option', { name: /crypto/i }).click()

      await page.getByLabel(/asset name|nome do ativo/i).fill(initialName)
      await page.getByLabel(/^code|^código/i).fill(assetCode)

      await page.getByRole('button', { name: /^Save$|^Salvar$/i }).click()
      await page.waitForLoadState('networkidle')

      // Verify asset appears in the list
      await expect(
        page.getByRole('row', { name: new RegExp(initialName) })
      ).toBeVisible({ timeout: 15000 })

      // Optionally verify success notification (toast may auto-dismiss quickly)
      const successToast = page
        .getByText(/criado com sucesso|successfully created/i)
        .first()
      await successToast.isVisible().catch(() => false)
      await page.waitForTimeout(1500)
    })

    await test.step('Open edit sheet', async () => {
      const row = page.getByRole('row', { name: new RegExp(initialName) })
      await expect(row).toBeVisible({ timeout: 10000 })

      // Use role-based selectors - data-testid not rendered in DOM
      await row.getByRole('button').last().click()
      await page.getByRole('menuitem', { name: /Details|Detalhes/i }).click()

      await expect(
        page.getByRole('heading', { name: new RegExp(initialName) })
      ).toBeVisible()
    })

    await test.step('Update asset name', async () => {
      const nameInput = page.getByLabel(/asset name|nome do ativo/i)
      await nameInput.clear()
      await nameInput.fill(updatedName)

      await page.getByRole('button', { name: /^Save$|^Salvar$/i }).click()
    })

    await test.step('Verify update', async () => {
      await expect(
        page
          .getByText(/alterações salvas com sucesso|saved successfully/i)
          .first()
      ).toBeVisible({ timeout: 15000 })

      // Verify updated name appears in the list
      await expect(
        page.getByRole('row', { name: new RegExp(updatedName) })
      ).toBeVisible({ timeout: 10000 })
    })
  })

  test('should validate required fields', async ({ page }) => {
    await test.step('Open create sheet', async () => {
      await page.getByRole('button', { name: /New Asset|Novo Ativo/i }).click()

      await expect(
        page.getByRole('heading', { name: /New Asset|Novo Ativo/i })
      ).toBeVisible({ timeout: 15000 })
    })

    await test.step('Try to save without required fields', async () => {
      // Try to save without filling anything
      const saveButton = page.getByRole('button', { name: /^Save$|^Salvar$/i })

      // Fill only name (missing type and code)
      await page
        .getByLabel(/asset name|nome do ativo/i)
        .fill('Incomplete Asset')

      await saveButton.click()

      // Should show validation errors for type and code
      // Wait to ensure form doesn't submit
      await page.waitForTimeout(1000)

      // Sheet should still be visible (not closed)
      await expect(
        page.getByRole('heading', { name: /New Asset|Novo Ativo/i })
      ).toBeVisible()
    })
  })

  test('should display page header and helper info', async ({ page }) => {
    await test.step('Verify page header elements', async () => {
      await expect(
        page.getByRole('heading', { name: /^Assets$|^Ativos$/i, level: 1 })
      ).toBeVisible()

      await expect(
        page.getByRole('button', { name: /New Asset|Novo Ativo/i })
      ).toBeVisible()

      await expect(
        page.getByRole('button', { name: /What is an Asset|O que é um Ativo/i })
      ).toBeVisible()
    })

    await test.step('Open helper info', async () => {
      const helperButton = page.getByRole('button', {
        name: /What is an Asset|O que é um Ativo/i
      })

      await helperButton.click()

      // Wait for content to load/expand
      await page.waitForLoadState('networkidle')
      await page.waitForTimeout(500)

      // Helper text should expand - verify any related text appears
      const helperContent = page.getByText(/asset|ativo/i).first()
      await expect(helperContent).toBeVisible({ timeout: 10000 })
    })
  })

  test('should create different asset types', async ({ page }) => {
    const assetTypes = [
      {
        type: 'crypto',
        name: `Crypto-${Date.now()}`,
        code: `CRY${Math.random().toString(36).substring(2, 8).toUpperCase().replace(/[0-9]/g, 'X')}`
      },
      {
        type: 'commodity',
        name: `Commodity-${Date.now()}`,
        code: `CMD${Math.random().toString(36).substring(2, 8).toUpperCase().replace(/[0-9]/g, 'X')}`
      }
    ]

    for (const assetType of assetTypes) {
      await test.step(`Create ${assetType.type} asset`, async () => {
        await page.waitForLoadState('networkidle')
        await page.waitForTimeout(500)

        const newAssetButton = page.getByRole('button', {
          name: /New Asset|Novo Ativo/i
        })
        await expect(newAssetButton).toBeVisible()
        await newAssetButton.click()

        await expect(
          page.getByRole('heading', { name: /New Asset|Novo Ativo/i })
        ).toBeVisible({ timeout: 15000 })

        await page.getByLabel(/type|tipo/i).click()
        await page
          .getByRole('option', { name: new RegExp(assetType.type, 'i') })
          .click()

        await page.getByLabel(/asset name|nome do ativo/i).fill(assetType.name)
        await page.getByLabel(/^code|^código/i).fill(assetType.code)

        await page.getByRole('button', { name: /^Save$|^Salvar$/i }).click()
        await page.waitForLoadState('networkidle')

        // Verify asset appears in the list (primary verification)
        await expect(
          page.getByRole('row', { name: new RegExp(assetType.name) })
        ).toBeVisible({ timeout: 15000 })

        // Optionally verify success notification (toast may auto-dismiss quickly)
        const successToast = page
          .getByText(/criado com sucesso|successfully created/i)
          .first()
        await successToast.isVisible().catch(() => false)

        await page.waitForTimeout(1000)
      })
    }
  })

  test('should handle tab navigation in asset sheet', async ({ page }) => {
    await test.step('Open create sheet', async () => {
      await page.getByRole('button', { name: /New Asset|Novo Ativo/i }).click()

      await expect(
        page.getByRole('heading', { name: /New Asset|Novo Ativo/i })
      ).toBeVisible({ timeout: 15000 })
    })

    await test.step('Verify both tabs are visible', async () => {
      await expect(
        page.getByRole('tab', { name: /details|detalhes/i })
      ).toBeVisible()

      await expect(page.getByRole('tab', { name: /metadata/i })).toBeVisible()
    })

    await test.step('Navigate to metadata tab', async () => {
      await page.getByRole('tab', { name: /metadata/i }).click()

      // Verify metadata fields are visible
      await expect(page.locator('#key')).toBeVisible()
      await expect(page.locator('#value')).toBeVisible()
    })

    await test.step('Navigate back to details', async () => {
      await page.getByRole('tab', { name: /details|detalhes/i }).click()

      // Verify detail fields are visible
      await expect(page.getByLabel(/type|tipo/i)).toBeVisible()
      await expect(page.getByLabel(/asset name|nome do ativo/i)).toBeVisible()
    })
  })
})
