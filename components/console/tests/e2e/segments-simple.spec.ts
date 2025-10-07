import { test, expect } from '@playwright/test'
import { navigateToSegments } from '../utils/navigate-to-segments'

/**
 * Segments - Simple E2E Tests
 *
 * This file contains the most basic, reliable tests for the Segments feature.
 * These tests focus on fundamental functionality without complex workflows.
 *
 * Key Principles:
 * 1. Use semantic selectors (getByRole, getByLabel, getByTestId)
 * 2. Wait for state changes, not arbitrary timeouts
 * 3. Test one thing at a time
 * 4. Use unique data to avoid conflicts
 */

test.describe('Segments - Basic Functionality', () => {
  test.beforeEach(async ({ page }) => {
    await navigateToSegments(page)
  })

  test('should display segments page with header and action button', async ({
    page
  }) => {
    // Verify page title
    await expect(
      page.getByRole('heading', { name: 'Segments', level: 1 })
    ).toBeVisible()

    // Verify "New Segment" button is present using role selector
    await expect(
      page.getByRole('button', { name: /new segment/i })
    ).toBeVisible()
  })

  test('should open create segment form sheet', async ({ page }) => {
    // Click the New Segment button
    // Note: This selector finds the button in either location:
    // 1. Header button (page.tsx:139) - always visible
    // 2. Empty state button (segments-data-table.tsx:121) - only when no segments exist
    await page.getByRole('button', { name: /new segment/i }).click()

    // Verify the sheet opened by checking for the heading
    await expect(
      page.getByRole('heading', { name: /new segment/i })
    ).toBeVisible()

    // Verify form elements are present
    await expect(page.getByLabel(/segment name/i)).toBeVisible()
    await expect(page.getByRole('button', { name: /^save$/i })).toBeVisible()
  })

  test('should create segment with minimal required fields', async ({
    page
  }) => {
    // Generate unique segment name
    const segmentName = `Test-Segment-${Date.now()}`

    // Step 1: Open the create form
    await page.getByRole('button', { name: /new segment/i }).click()
    await expect(
      page.getByRole('heading', { name: /new segment/i })
    ).toBeVisible()

    // Step 2: Fill the required segment name field
    const nameInput = page.getByLabel(/segment name/i)
    await nameInput.fill(segmentName)
    // Ensure the input loses focus to trigger validation
    await nameInput.blur()
    await page.waitForTimeout(300)

    // Step 3: Submit the form
    const saveButton = page.getByRole('button', { name: /^save$/i })
    await expect(saveButton).toBeEnabled()
    await saveButton.click()

    // Wait for the API request to complete (indicated by button state change or loading)
    await page.waitForTimeout(1000)

    // Step 4: Wait for the sheet to close (indicates successful creation)
    await expect(
      page.getByRole('heading', { name: /new segment/i })
    ).not.toBeVisible({ timeout: 15000 })

    // Wait for potential refetch/reload after creation
    await page.waitForTimeout(2000)

    // Step 5: Verify the segment appears in the table
    // Check if the segment row with the name exists in the table
    const segmentRow = page.getByRole('row', {
      name: new RegExp(segmentName, 'i')
    })

    // Wait for the row to appear, with extended timeout for API/UI updates
    await expect(segmentRow).toBeVisible({ timeout: 15000 })
  })

  test('should validate required segment name field', async ({ page }) => {
    // Open the create form
    await page.getByRole('button', { name: /new segment/i }).click()
    await expect(
      page.getByRole('heading', { name: /new segment/i })
    ).toBeVisible()

    // Try to submit without filling the name field
    await page.getByRole('button', { name: /^save$/i }).click()

    // Verify validation error message appears
    await expect(
      page.locator('text=/.*name.*(required|must be at least)/i')
    ).toBeVisible()
  })

  test('should create segment with metadata', async ({ page }) => {
    // Generate unique segment name
    const segmentName = `Meta-Segment-${Date.now()}`

    // Step 1: Open the create form
    await page.getByRole('button', { name: /new segment/i }).click()
    await expect(
      page.getByRole('heading', { name: /new segment/i })
    ).toBeVisible()

    // Step 2: Fill the segment name
    await page.getByLabel(/segment name/i).fill(segmentName)

    // Step 3: Switch to Metadata tab
    await page.getByRole('tab', { name: /metadata/i }).click()

    // Wait briefly for tab content to render
    await page.waitForTimeout(300)

    // Step 4: Add metadata key-value pair
    await page.locator('#key').fill('category')
    await page.locator('#value').fill('retail')

    // Click the add metadata button (+ icon button - last button with SVG)
    const addButton = page
      .locator('button')
      .filter({
        has: page.locator('svg')
      })
      .last()
    await addButton.click()

    // Step 5: Submit the form
    await page.getByRole('button', { name: /^save$/i }).click()

    // Step 6: Wait for sheet to close
    await expect(
      page.getByRole('heading', { name: /new segment/i })
    ).not.toBeVisible({ timeout: 10000 })

    // Step 7: Verify segment appears in table
    await expect(
      page.getByRole('row', { name: new RegExp(segmentName, 'i') })
    ).toBeVisible({ timeout: 15000 })
  })
})

test.describe('Segments - CRUD Operations', () => {
  test.beforeEach(async ({ page }) => {
    await navigateToSegments(page)
  })

  test('should edit an existing segment', async ({ page }) => {
    // Step 1: Create a segment to edit
    const originalName = `Edit-Test-${Date.now()}`
    const updatedName = `Updated-${Date.now()}`

    await page.getByRole('button', { name: /new segment/i }).click()
    await expect(
      page.getByRole('heading', { name: /new segment/i })
    ).toBeVisible()

    await page.getByLabel(/segment name/i).fill(originalName)
    await page.getByRole('button', { name: /^save$/i }).click()

    await expect(
      page.getByRole('heading', { name: /new segment/i })
    ).not.toBeVisible({ timeout: 10000 })

    // Step 2: Wait for segment to appear
    const segmentRow = page.getByRole('row', {
      name: new RegExp(originalName, 'i')
    })
    await expect(segmentRow).toBeVisible({ timeout: 15000 })

    // Step 3: Open the actions menu and click Details
    await segmentRow.getByRole('button').last().click()
    await page.getByRole('menuitem', { name: /details/i }).click()

    // Step 4: Verify edit sheet opened
    await expect(
      page.getByRole('heading', {
        name: new RegExp(`edit.*${originalName}`, 'i')
      })
    ).toBeVisible()

    // Step 5: Update the segment name
    const nameInput = page.getByLabel(/segment name/i)
    await nameInput.clear()
    await nameInput.fill(updatedName)

    // Step 6: Save changes
    await page.getByRole('button', { name: /^save$/i }).click()

    // Step 7: Wait for sheet to close
    await expect(page.getByRole('heading', { name: /edit/i })).not.toBeVisible({
      timeout: 10000
    })

    // Step 8: Verify updated segment appears in table
    await expect(
      page.getByRole('row', { name: new RegExp(updatedName, 'i') })
    ).toBeVisible({ timeout: 15000 })
  })

  test('should delete a segment with confirmation', async ({ page }) => {
    // Step 1: Create a segment to delete
    const segmentName = `Delete-Test-${Date.now()}`

    await page.getByRole('button', { name: /new segment/i }).click()
    await expect(
      page.getByRole('heading', { name: /new segment/i })
    ).toBeVisible()

    await page.getByLabel(/segment name/i).fill(segmentName)
    await page.getByRole('button', { name: /^save$/i }).click()

    await expect(
      page.getByRole('heading', { name: /new segment/i })
    ).not.toBeVisible({ timeout: 10000 })

    // Step 2: Wait for segment to appear
    const segmentRow = page.getByRole('row', {
      name: new RegExp(segmentName, 'i')
    })
    await expect(segmentRow).toBeVisible({ timeout: 15000 })

    // Step 3: Open actions menu and click Delete
    await segmentRow.getByRole('button').last().click()
    await page.getByRole('menuitem', { name: /delete/i }).click()

    // Step 4: Confirm deletion in the dialog
    await page.getByRole('button', { name: /confirm/i }).click()

    // Step 5: Verify segment is removed from table
    await expect(segmentRow).not.toBeVisible({ timeout: 10000 })
  })
})
