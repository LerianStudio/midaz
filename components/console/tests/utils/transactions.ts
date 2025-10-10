import { Page } from '@playwright/test'
import { inputType, selectOption } from './form'

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
  // Wait for the metadata accordion to be visible
  const accordion = page.getByTestId('metadata-accordion')
  await accordion.waitFor({ state: 'visible', timeout: 5000 })

  // Click the chevron trigger to expand the accordion
  const trigger = accordion.getByTestId('paper-collapsible-trigger')
  await trigger.click()
  await page.waitForTimeout(500)

  // Fill each metadata key-value pair
  for (const [key, value] of Object.entries(metadata)) {
    // Fill the key input
    const keyInput = page.getByTestId('metadata-key-input')
    await keyInput.waitFor({ state: 'visible', timeout: 5000 })
    await keyInput.fill(key)

    // Fill the value input
    const valueInput = page.getByTestId('metadata-value-input')
    await valueInput.fill(value)

    // Click the add button
    const addButton = page.getByTestId('metadata-add-button')
    await addButton.click()
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

  // Fill metadata if provided
  if (data.metadata && Object.keys(data.metadata).length > 0) {
    await fillMetadata(page, data.metadata)
  }

  // Wait for Review button to appear
  const reviewButton = page.getByTestId('transaction-review-button')
  await reviewButton.waitFor({ state: 'visible', timeout: 15000 })
}
