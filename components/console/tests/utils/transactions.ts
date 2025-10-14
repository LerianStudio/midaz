import { Page } from '@playwright/test'
import { inputType, selectOption } from './form'
import { expandAccordion } from './accordion'
import { inputMetadata } from './metadata'

interface SimpleTransactionData {
  description: string
  asset: string
  value: string
  sourceAccount: string
  destinationAccount: string
  metadata?: Record<string, string>
}

interface ComplexTransactionData {
  description: string
  asset: string
  value: string
  sourceAccounts: readonly string[]
  destinationAccounts: readonly string[]
  sourceAmounts?: readonly string[]
  destinationAmounts?: readonly string[]
  metadata?: Record<string, string>
}

export async function fillSimpleTransaction(
  page: Page,
  data: SimpleTransactionData
) {
  // Fill basic information
  await inputType(page, 'transaction-description', data.description)

  // Select asset
  const assetSelected = await selectOption(
    page,
    'transaction-asset',
    data.asset
  )
  if (!assetSelected) {
    throw new Error(`Failed to select asset: ${data.asset}`)
  }

  // Fill value
  await inputType(page, 'transaction-value', data.value)

  // Click Next button to proceed to source/destination step
  await page.getByTestId('transaction-next-button').click()
  await page.waitForTimeout(500)

  // Fill source account
  await page
    .getByTestId('source-account-search')
    .waitFor({ state: 'visible', timeout: 5000 })

  // Click to open the autocomplete dropdown
  await page.getByTestId('source-account-search').click()
  await page.waitForTimeout(300)

  // Type in the search field for source account
  const sourceInput = page.getByTestId('source-account-search')
  await sourceInput.fill(data.sourceAccount)
  await page.waitForTimeout(1500) // Wait for debounce (500ms) and API response

  // Wait for and click the cmdk item
  const sourceOption = page
    .locator('[cmdk-item]')
    .filter({ hasText: data.sourceAccount })
    .first()
  await sourceOption.waitFor({ state: 'visible', timeout: 10000 })
  await sourceOption.click()
  await page.waitForTimeout(300)

  // Click add button for source
  const addSourceButton = page.getByTestId('source-account-search-add-button')
  await addSourceButton.waitFor({ state: 'visible', timeout: 5000 })
  await addSourceButton.click()
  await page.waitForTimeout(500)

  // Fill destination account
  await page
    .getByTestId('destination-account-search')
    .waitFor({ state: 'visible', timeout: 5000 })

  // Click to open the autocomplete dropdown
  await page.getByTestId('destination-account-search').click()
  await page.waitForTimeout(300)

  // Type in the search field for destination account
  const destinationInput = page.getByTestId('destination-account-search')
  await destinationInput.fill(data.destinationAccount)
  await page.waitForTimeout(1500) // Wait for debounce and API response

  // Wait for and click the cmdk item
  const destinationOption = page
    .locator('[cmdk-item]')
    .filter({ hasText: data.destinationAccount })
    .first()
  await destinationOption.waitFor({ state: 'visible', timeout: 10000 })
  await destinationOption.click()
  await page.waitForTimeout(300)

  // Click add button for destination
  const addDestinationButton = page.getByTestId(
    'destination-account-search-add-button'
  )
  await addDestinationButton.waitFor({ state: 'visible', timeout: 5000 })
  await addDestinationButton.click()
  await page.waitForTimeout(500)

  // Click Next again to proceed to operations step (step 2)
  await page.getByTestId('transaction-next-button').click()
  await page.waitForTimeout(1000)

  // Fill metadata if provided
  if (data.metadata && Object.keys(data.metadata).length > 0) {
    await fillMetadata(page, data.metadata)
  }

  // Wait for Review button to appear
  const reviewButton = page.getByTestId('transaction-review-button')
  await reviewButton.waitFor({ state: 'visible', timeout: 15000 })
}

export async function fillMetadata(
  page: Page,
  metadata: Record<string, string>
) {
  // Expand the metadata accordion
  await expandAccordion(page, 'metadata-accordion')

  // Input the metadata key-value pairs
  await inputMetadata(page, metadata)
}

/**
 * Update amounts for multiple operations
 * This is used when splitting a transaction across multiple accounts
 * @param page - Playwright page object
 * @param amounts - Array of amounts to fill
 * @param startIndex - Starting index for the operation value inputs (0 for sources, sourceCount for destinations)
 */
export async function updateOperationAmounts(
  page: Page,
  amounts: readonly string[],
  startIndex: number = 0
) {
  const valueInputs = page.getByTestId('operation-value-input')
  const count = await valueInputs.count()

  // Verify we have enough inputs starting from startIndex
  if (startIndex + amounts.length > count) {
    throw new Error(
      `Expected at least ${startIndex + amounts.length} operation value inputs, found ${count}`
    )
  }

  for (let i = 0; i < amounts.length; i++) {
    const input = valueInputs.nth(startIndex + i)
    await input.waitFor({ state: 'visible', timeout: 5000 })
    await input.clear()
    await input.fill(amounts[i])
    await page.waitForTimeout(300)
  }
}

export async function fillComplexTransaction(
  page: Page,
  data: ComplexTransactionData
) {
  // Fill basic information
  await inputType(page, 'transaction-description', data.description)

  // Select asset
  const assetSelected = await selectOption(
    page,
    'transaction-asset',
    data.asset
  )
  if (!assetSelected) {
    throw new Error(`Failed to select asset: ${data.asset}`)
  }

  // Fill value
  await inputType(page, 'transaction-value', data.value)

  // Click Next button to proceed to source/destination step
  await page.getByTestId('transaction-next-button').click()
  await page.waitForTimeout(500)

  // Fill source accounts
  for (const sourceAccount of data.sourceAccounts) {
    await page
      .getByTestId('complex-source-account-search')
      .waitFor({ state: 'visible', timeout: 5000 })

    const sourceInput = page.getByTestId('complex-source-account-search')
    await sourceInput.click()
    await page.waitForTimeout(300)

    await sourceInput.fill(sourceAccount)
    await page.waitForTimeout(1500) // Wait for debounce and API response

    // Wait for and click the cmdk item
    const sourceOption = page
      .locator('[cmdk-item]')
      .filter({ hasText: sourceAccount })
      .first()
    await sourceOption.waitFor({ state: 'visible', timeout: 10000 })
    await sourceOption.click()
    await page.waitForTimeout(300)

    // Click add button for source
    const addSourceButton = page.getByTestId(
      'complex-source-account-search-add-button'
    )
    await addSourceButton.waitFor({ state: 'visible', timeout: 5000 })
    await addSourceButton.click()
    await page.waitForTimeout(500)
  }

  // Fill destination accounts
  for (const destinationAccount of data.destinationAccounts) {
    await page
      .getByTestId('complex-destination-account-search')
      .waitFor({ state: 'visible', timeout: 5000 })

    const destinationInput = page.getByTestId(
      'complex-destination-account-search'
    )
    await destinationInput.click()
    await page.waitForTimeout(300)

    await destinationInput.fill(destinationAccount)
    await page.waitForTimeout(1500) // Wait for debounce and API response

    // Wait for and click the cmdk item
    const destinationOption = page
      .locator('[cmdk-item]')
      .filter({ hasText: destinationAccount })
      .first()
    await destinationOption.waitFor({ state: 'visible', timeout: 10000 })
    await destinationOption.click()
    await page.waitForTimeout(300)

    // Click add button for destination
    const addDestinationButton = page.getByTestId(
      'complex-destination-account-search-add-button'
    )
    await addDestinationButton.waitFor({ state: 'visible', timeout: 5000 })
    await addDestinationButton.click()
    await page.waitForTimeout(500)
  }

  // Click Next again to proceed to operations step (step 2)
  await page.getByTestId('transaction-next-button').click()
  await page.waitForTimeout(1000)

  // Update source amounts if provided (for multiple sources)
  if (data.sourceAmounts && data.sourceAmounts.length > 0) {
    await updateOperationAmounts(page, data.sourceAmounts, 0)
  }

  // Update destination amounts if provided (for multiple destinations)
  // Destination inputs come after source inputs, but only if sources have editable inputs (length > 1)
  if (data.destinationAmounts && data.destinationAmounts.length > 0) {
    // Only count editable source inputs (sources are editable only when there are multiple sources)
    const editableSourceCount =
      data.sourceAccounts.length > 1 ? data.sourceAccounts.length : 0
    await updateOperationAmounts(
      page,
      data.destinationAmounts,
      editableSourceCount
    )
  }

  // Fill metadata if provided
  if (data.metadata && Object.keys(data.metadata).length > 0) {
    await fillMetadata(page, data.metadata)
  }

  // Wait for Review button to appear
  const reviewButton = page.getByTestId('transaction-review-button')
  await reviewButton.waitFor({ state: 'visible', timeout: 15000 })
}
