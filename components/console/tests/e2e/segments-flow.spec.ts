import { test, expect } from '@playwright/test'
import { navigateToSegments } from '../utils/navigate-to-segments'

test.beforeEach(async ({ page }) => {
  await navigateToSegments(page)
})

test.describe('Segments Management - E2E Tests', () => {
  test('should navigate to segments page and verify page loads', async ({
    page
  }) => {
    await test.step('Verify page header elements', async () => {
      // Verify main heading is visible
      await expect(
        page.getByRole('heading', { name: /^Segments$/i, level: 1 })
      ).toBeVisible()

      // Verify "New Segment" button is visible in header
      await expect(page.getByTestId('new-segment').first()).toBeVisible()

      // Verify helper info button is visible
      await expect(
        page.getByRole('button', { name: /What is a Segment/i })
      ).toBeVisible()
    })

    await test.step('Verify search and pagination controls', async () => {
      // Verify search by ID field is visible
      await expect(page.getByPlaceholder(/Search by ID/i)).toBeVisible()

      // Verify pagination limit field is visible
      await expect(page.getByTestId('pagination-limit')).toBeVisible()
    })
  })

  test('should verify empty state when no segments exist', async ({ page }) => {
    await test.step('Check for empty state or existing segments', async () => {
      // Wait for page to be stable
      await page.waitForLoadState('networkidle')
      await page.waitForTimeout(500)

      // Check if empty state is visible or if segments exist
      const emptyStateText = page.getByText(
        /You haven't created any Segments yet/i
      )
      const isEmptyState = await emptyStateText.isVisible().catch(() => false)

      if (isEmptyState) {
        // Verify empty state message
        await expect(emptyStateText).toBeVisible()

        // Verify "New Segment" button in empty state
        await expect(page.getByTestId('new-segment').last()).toBeVisible()
      } else {
        // Wait for data to load
        await page.waitForLoadState('networkidle')
        await page.waitForTimeout(500)

        // Verify table exists first
        await expect(page.getByTestId('segments-table')).toBeVisible()

        // Segments exist - verify table header is visible
        await expect(
          page.getByRole('columnheader', { name: /^Name$/i })
        ).toBeVisible()
      }
    })
  })

  test('should create a new segment with basic details', async ({ page }) => {
    const segmentName = `E2E-Segment-${Date.now()}`

    await test.step('Set pagination limit to show more items', async () => {
      // Increase pagination limit to 100 to ensure new segment is visible
      const limitSelect = page.getByTestId('pagination-limit')
      await limitSelect.click()
      await page.getByRole('option', { name: '100' }).click()
      await page.waitForLoadState('networkidle')
      await page.waitForTimeout(500)
    })

    await test.step('Open create segment sheet', async () => {
      await page.waitForLoadState('networkidle')
      await page.waitForTimeout(500)

      // Click "New Segment" button (use first() to get header button)
      const newSegmentButton = page.getByTestId('new-segment').first()
      await expect(newSegmentButton).toBeVisible()
      await newSegmentButton.click()

      // Wait for sheet to open by checking for the visible heading
      await expect(
        page.getByRole('heading', { name: /New Segment/i })
      ).toBeVisible({ timeout: 15000 })
    })

    await test.step('Fill in segment name', async () => {
      // Fill in segment name
      await page.getByLabel(/Segment Name/i).fill(segmentName)

      // Wait for form validation
      await page.waitForTimeout(1000)

      // Ensure save button is enabled
      const saveButton = page.getByRole('button', { name: /^Save$|^Salvar$/i })
      await expect(saveButton).toBeEnabled({ timeout: 5000 })
    })

    await test.step('Save the segment', async () => {
      // Save the segment
      const saveButton = page.getByRole('button', { name: /^Save$|^Salvar$/i })
      await expect(saveButton).toBeVisible()
      await expect(saveButton).toBeEnabled()
      await saveButton.click()

      // Wait for sheet to close
      await expect(
        page.getByRole('heading', { name: /New Segment/i })
      ).not.toBeVisible({ timeout: 10000 })

      // Wait for save operation to complete
      await page.waitForLoadState('networkidle')
    })

    await test.step('Verify segment appears in the list', async () => {
      // Wait for any refetch/loading to complete
      await page.waitForTimeout(2000)
      await page.waitForLoadState('networkidle')

      // Reload page to ensure fresh data
      await page.reload({ waitUntil: 'networkidle' })
      await page.waitForTimeout(1000)

      // Verify segment appears in the table (check if it exists anywhere on page)
      const segmentRow = page.getByRole('row', {
        name: new RegExp(segmentName)
      })
      const isVisible = await segmentRow.isVisible().catch(() => false)

      if (!isVisible) {
        // If not visible, it might be on another page or not created
        // Check if table has any rows at all
        const tableBody = page.locator('tbody')
        const rowCount = await tableBody.locator('tr').count()

        // Log for debugging
        console.log(`Rows in table: ${rowCount}`)

        // Try waiting a bit more and check again
        await page.waitForTimeout(2000)
        await expect(segmentRow).toBeVisible({ timeout: 10000 })
      } else {
        await expect(segmentRow).toBeVisible()
      }

      // Optionally verify success notification
      const successToast = page
        .getByText(/criado com sucesso|successfully created/i)
        .first()
      await successToast.isVisible().catch(() => false)
    })
  })

  test('should create segment with metadata', async ({ page }) => {
    const segmentName = `Full-Segment-${Date.now()}`

    await test.step('Set pagination limit to show more items', async () => {
      const limitSelect = page.getByTestId('pagination-limit')
      await limitSelect.click()
      await page.getByRole('option', { name: '100' }).click()
      await page.waitForLoadState('networkidle')
      await page.waitForTimeout(500)
    })

    await test.step('Open create segment sheet', async () => {
      await page.waitForLoadState('networkidle')
      await page.waitForTimeout(500)

      const newSegmentButton = page.getByTestId('new-segment').first()
      await expect(newSegmentButton).toBeVisible()
      await newSegmentButton.click()

      await expect(
        page.getByRole('heading', { name: /New Segment/i })
      ).toBeVisible({ timeout: 15000 })
    })

    await test.step('Fill basic details', async () => {
      await page.getByLabel(/Segment Name/i).fill(segmentName)

      // Wait for form validation
      await page.waitForTimeout(1000)
    })

    await test.step('Add metadata', async () => {
      // Navigate to Metadata tab
      await page.getByRole('tab', { name: /metadata/i }).click()
      await page.waitForTimeout(500)

      // Verify metadata fields are visible
      await expect(page.locator('#key')).toBeVisible()
      await expect(page.locator('#value')).toBeVisible()

      // Add metadata entry
      await page.locator('#key').fill('environment')
      await page.locator('#value').fill('test')

      // Look for "Add" button to add the metadata entry
      const addButton = page
        .getByRole('button', { name: /Add|Adicionar/i })
        .first()
      if (await addButton.isVisible().catch(() => false)) {
        await addButton.click()
        await page.waitForTimeout(300)
      }

      // Navigate back to details tab
      await page.getByRole('tab', { name: /details|detalhes/i }).click()
      await page.waitForTimeout(300)
    })

    await test.step('Save and verify', async () => {
      const saveButton = page.getByRole('button', { name: /^Save$|^Salvar$/i })
      await expect(saveButton).toBeVisible()
      await expect(saveButton).toBeEnabled()
      await page.waitForTimeout(500)
      await saveButton.click()

      // Wait for sheet to close
      await expect(
        page.getByRole('heading', { name: /New Segment/i })
      ).not.toBeVisible({ timeout: 10000 })

      await page.waitForLoadState('networkidle')

      // Wait for any refetch/loading to complete
      await page.waitForTimeout(2000)

      // Verify segment appears in the list
      await expect(
        page.getByRole('row', { name: new RegExp(segmentName) })
      ).toBeVisible({ timeout: 15000 })
    })
  })

  test('should edit an existing segment', async ({ page }) => {
    const initialName = `Edit-Segment-${Date.now()}`
    const updatedName = `Updated-${Date.now()}`

    await test.step('Set pagination limit to show more items', async () => {
      const limitSelect = page.getByTestId('pagination-limit')
      await limitSelect.click()
      await page.getByRole('option', { name: '100' }).click()
      await page.waitForLoadState('networkidle')
      await page.waitForTimeout(500)
    })

    await test.step('Create segment first', async () => {
      await page.waitForLoadState('networkidle')
      await page.waitForTimeout(500)

      const newSegmentButton = page.getByTestId('new-segment').first()
      await expect(newSegmentButton).toBeVisible()
      await newSegmentButton.click()

      await expect(
        page.getByRole('heading', { name: /New Segment/i })
      ).toBeVisible({ timeout: 15000 })

      await page.getByLabel(/Segment Name/i).fill(initialName)

      // Wait for form validation
      await page.waitForTimeout(1000)

      // Ensure save button is enabled
      const saveButton = page.getByRole('button', { name: /^Save$|^Salvar$/i })
      await expect(saveButton).toBeEnabled({ timeout: 5000 })
      await saveButton.click()

      await page.waitForLoadState('networkidle')

      // Wait for any refetch/loading to complete
      await page.waitForTimeout(2000)

      // Verify segment appears in the list
      await expect(
        page.getByRole('row', { name: new RegExp(initialName) })
      ).toBeVisible({ timeout: 15000 })

      await page.waitForTimeout(1500)
    })

    await test.step('Open edit sheet', async () => {
      const row = page.getByRole('row', { name: new RegExp(initialName) })
      await expect(row).toBeVisible({ timeout: 10000 })

      // Open actions dropdown - use last button (three-dot menu)
      await row.getByRole('button').last().click()

      // Click "Details" option to edit
      await page.getByRole('menuitem', { name: /Details|Detalhes/i }).click()

      // Verify edit sheet is open with segment name in heading
      await expect(
        page.getByRole('heading', { name: new RegExp(initialName) })
      ).toBeVisible()
    })

    await test.step('Update segment name', async () => {
      const nameInput = page.getByLabel(/Segment Name/i)
      await nameInput.clear()
      await nameInput.fill(updatedName)

      await page.getByRole('button', { name: /^Save$|^Salvar$/i }).click()
    })

    await test.step('Verify update', async () => {
      // Verify success message
      await expect(
        page
          .getByText(/altera��es salvas com sucesso|saved successfully/i)
          .first()
      ).toBeVisible({ timeout: 15000 })

      // Verify updated name appears in the list
      await expect(
        page.getByRole('row', { name: new RegExp(updatedName) })
      ).toBeVisible({ timeout: 10000 })
    })
  })

  test('should delete a segment', async ({ page }) => {
    const segmentName = `Delete-Segment-${Date.now()}`

    await test.step('Set pagination limit to show more items', async () => {
      const limitSelect = page.getByTestId('pagination-limit')
      await limitSelect.click()
      await page.getByRole('option', { name: '100' }).click()
      await page.waitForLoadState('networkidle')
      await page.waitForTimeout(500)
    })

    await test.step('Create segment to delete', async () => {
      await page.waitForLoadState('networkidle')
      await page.waitForTimeout(500)

      const newSegmentButton = page.getByTestId('new-segment').first()
      await expect(newSegmentButton).toBeVisible()
      await newSegmentButton.click()

      await expect(
        page.getByRole('heading', { name: /New Segment/i })
      ).toBeVisible({ timeout: 15000 })

      await page.getByLabel(/Segment Name/i).fill(segmentName)

      // Wait for form validation
      await page.waitForTimeout(1000)

      // Ensure save button is enabled
      const saveButton = page.getByRole('button', { name: /^Save$|^Salvar$/i })
      await expect(saveButton).toBeEnabled({ timeout: 5000 })
      await saveButton.click()

      await page.waitForLoadState('networkidle')

      // Wait for any refetch/loading to complete
      await page.waitForTimeout(2000)

      // Verify segment appears in the list
      await expect(
        page.getByRole('row', { name: new RegExp(segmentName) })
      ).toBeVisible({ timeout: 15000 })

      await page.waitForTimeout(1000)
    })

    await test.step('Delete the segment', async () => {
      // Locate the segment row
      const testSegmentRow = page.getByRole('row', {
        name: new RegExp(segmentName)
      })

      // Wait for stable state
      await page.waitForLoadState('networkidle')

      // Open actions dropdown - use the last button in the row (three-dot menu)
      await testSegmentRow.getByRole('button').last().click()

      // Select delete option
      await page.getByRole('menuitem', { name: /Delete|Deletar/i }).click()

      // Confirm deletion
      await page.getByRole('button', { name: /Confirm|Confirmar/i }).click()

      // Wait for deletion to complete
      await page.waitForLoadState('networkidle')

      // Verify success notification
      await expect(
        page.getByText(/exclu�do com sucesso|successfully deleted/i).first()
      ).toBeVisible({ timeout: 10000 })
    })

    await test.step('Verify segment is removed from list', async () => {
      // Verify segment no longer appears in the list
      const deletedRow = page.getByRole('row', {
        name: new RegExp(segmentName)
      })
      await expect(deletedRow).not.toBeVisible({ timeout: 5000 })
    })
  })

  test('should validate required fields', async ({ page }) => {
    await test.step('Open create sheet', async () => {
      await page.getByTestId('new-segment').first().click()

      await expect(
        page.getByRole('heading', { name: /New Segment/i })
      ).toBeVisible({ timeout: 15000 })
    })

    await test.step('Try to save without required fields', async () => {
      // Try to save without filling segment name
      const saveButton = page.getByRole('button', { name: /^Save$|^Salvar$/i })

      await saveButton.click()

      // Wait to ensure form doesn't submit
      await page.waitForTimeout(1000)

      // Sheet should still be visible (not closed)
      await expect(
        page.getByRole('heading', { name: /New Segment/i })
      ).toBeVisible()
    })
  })

  test('should handle tab navigation in segment sheet', async ({ page }) => {
    await test.step('Open create sheet', async () => {
      await page.getByTestId('new-segment').first().click()

      await expect(
        page.getByRole('heading', { name: /New Segment/i })
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
      await expect(page.getByLabel(/Segment Name/i)).toBeVisible()
    })
  })

  test('should filter segments by ID', async ({ page }) => {
    const segmentName = `Search-Segment-${Date.now()}`
    let segmentId = ''

    await test.step('Set pagination limit to show more items', async () => {
      const limitSelect = page.getByTestId('pagination-limit')
      await limitSelect.click()
      await page.getByRole('option', { name: '100' }).click()
      await page.waitForLoadState('networkidle')
      await page.waitForTimeout(500)
    })

    await test.step('Create a segment to search for', async () => {
      await page.waitForLoadState('networkidle')
      await page.waitForTimeout(500)

      const newSegmentButton = page.getByTestId('new-segment').first()
      await expect(newSegmentButton).toBeVisible()
      await newSegmentButton.click()

      await expect(
        page.getByRole('heading', { name: /New Segment/i })
      ).toBeVisible({ timeout: 15000 })

      await page.getByLabel(/Segment Name/i).fill(segmentName)

      // Wait for form validation
      await page.waitForTimeout(1000)

      // Ensure save button is enabled
      const saveButton = page.getByRole('button', { name: /^Save$|^Salvar$/i })
      await expect(saveButton).toBeEnabled({ timeout: 5000 })
      await saveButton.click()

      await page.waitForLoadState('networkidle')

      // Wait for any refetch/loading to complete
      await page.waitForTimeout(2000)

      // Verify segment appears in the list
      const row = page.getByRole('row', { name: new RegExp(segmentName) })
      await expect(row).toBeVisible({ timeout: 15000 })

      // Extract segment ID from the row (it's displayed in the ID column)
      const idCell = row.locator('td').nth(1) // ID is the second column
      const idText = await idCell.textContent()

      // Extract just the ID (may be truncated in UI, so get first few chars)
      if (idText) {
        segmentId = idText.trim().substring(0, 8)
      }

      await page.waitForTimeout(1000)
    })

    await test.step('Search by segment ID', async () => {
      if (segmentId) {
        // Enter search query
        const searchInput = page.getByPlaceholder(/Search by ID/i)
        await expect(searchInput).toBeVisible()
        await searchInput.fill(segmentId)

        // Wait for search to filter results
        await page.waitForLoadState('networkidle')
        await page.waitForTimeout(500)

        // Verify segment is still visible (or filtered results show it)
        // Note: Depending on exact match vs partial match implementation
        const row = page.getByRole('row', { name: new RegExp(segmentName) })
        await expect(row).toBeVisible({ timeout: 5000 })
      }
    })

    await test.step('Clear search', async () => {
      const searchInput = page.getByPlaceholder(/Search by ID/i)
      await searchInput.clear()
      await page.waitForLoadState('networkidle')
    })
  })

  test('should display helper information', async ({ page }) => {
    await test.step('Open helper info', async () => {
      const helperButton = page.getByRole('button', {
        name: /What is a Segment/i
      })

      await helperButton.click()

      // Wait for content to load/expand
      await page.waitForLoadState('networkidle')
      await page.waitForTimeout(500)

      // Verify helper content appears
      const helperText = page.getByText(/Custom labels that allow grouping/i)
      await expect(helperText).toBeVisible({ timeout: 10000 })
    })
  })
})
