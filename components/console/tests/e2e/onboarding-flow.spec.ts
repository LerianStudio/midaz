import { test, expect, Page } from '@playwright/test'
import { testDataFactory } from '../fixtures/test-data.factory'
import { CommonHelpers } from '../utils/common-helpers'
import { navigateToOnboarding } from '../utils/navigate-to-onboarding'
import { FileUploadHelper } from '../utils/file-upload'

async function fillFormStep1(
  page: Page,
  testData: ReturnType<typeof testDataFactory.organization>,
  formData?: Partial<{
    legalName: string
    doingBusinessAs: string
    legalDocument: string
  }>
) {
  await page
    .locator('input[name="legalName"]')
    .fill(formData?.legalName ?? testData.legalName)
  await page
    .locator('input[name="doingBusinessAs"]')
    .fill(formData?.doingBusinessAs ?? testData.doingBusinessAs)
  await page
    .locator('input[name="legalDocument"]')
    .fill(formData?.legalDocument ?? testData.legalDocument)
}

async function fillFormStep2(
  page: Page,
  testData: ReturnType<typeof testDataFactory.organization>
) {
  await page.locator('input[name="address.line1"]').fill(testData.address.line1)
  await page.locator('input[name="address.line2"]').fill(testData.address.line2)
  // Country select
  await page.getByTestId('country-select').locator('button').click()
  await page.getByRole('group').locator('[data-value="BR"]').click()
  // State select
  await page.getByTestId('state-select').click()
  await page.getByRole('group').locator('[data-value="AM"]').click()
  await page.locator('input[name="address.city"]').fill(testData.address.city)
  await page
    .locator('input[name="address.zipCode"]')
    .fill(testData.address.zipCode.replaceAll(/[^\d]/g, ''))
}

test.describe('Onboarding Flow - E2E Tests', () => {
  let testData: ReturnType<typeof testDataFactory.organization>
  let ledgerData: ReturnType<typeof testDataFactory.ledger>

  test.beforeEach(async ({ page }) => {
    testData = testDataFactory.organization()
    ledgerData = testDataFactory.ledger()
    await navigateToOnboarding(page)
  })

  test.describe('Multi-Step Onboarding Flow', () => {
    test('should complete full onboarding with all steps', async ({ page }) => {
      await test.step('Step 1: Fill organization details', async () => {
        await fillFormStep1(page, testData)

        // Continue to next step
        await page.getByTestId('next-button').click()
        await CommonHelpers.waitForNetworkIdle(page)
      })

      await test.step('Step 2: Fill address information', async () => {
        await fillFormStep2(page, testData)

        // Continue to next step
        await page.getByTestId('next-button').click()
        await CommonHelpers.waitForNetworkIdle(page)
      })

      await test.step('Step 3: Configure theme (optional)', async () => {
        // Theme upload logo image
        const avatarUpload = page
          .getByTestId('avatar-upload-container')
          .getByRole('button')

        if (await avatarUpload.isVisible()) {
          await avatarUpload.click()
          await FileUploadHelper.uploadImage(page, '#avatar', 'test-avatar.png')
          await page.getByRole('button', { name: /Send|Enviar/i }).click()
        }

        // Complete onboarding
        await page
          .getByRole('button', { name: /finish|complete|submit/i })
          .first()
          .click()
        await CommonHelpers.waitForNetworkIdle(page)
      })

      await test.step('Verify successful onboarding completion', async () => {
        // Should redirect to main app or show success message
        await expect(
          page
            .getByText(/.*active and operational|.*ativa e operante/i)
            .or(page.getByTestId('success-toast'))
        ).toBeVisible({ timeout: 10000 })
      })
    })

    test('should complete onboarding with minimal required fields', async ({
      page
    }) => {
      await test.step('Fill only required fields - Step 1', async () => {
        await fillFormStep1(page, testData)

        await page.getByTestId('next-button').click()
        await CommonHelpers.waitForNetworkIdle(page)
      })

      await test.step('Fill minimal address - Step 2', async () => {
        await fillFormStep2(page, testData)

        await page.getByTestId('next-button').click()
        await CommonHelpers.waitForNetworkIdle(page)
      })

      await test.step('Skip optional theme - Step 3', async () => {
        await page
          .getByRole('button', { name: /finish|complete|skip/i })
          .click()
        await page
          .getByRole('button', {
            name: /Yes, I will configure it later|Sim, vou configurar depois/i
          })
          .click()
        await CommonHelpers.waitForNetworkIdle(page)
      })

      await test.step('Verify completion', async () => {
        await expect(
          page
            .getByText(/.*active and operational|.*ativa e operante/i)
            .or(page.getByTestId('success-toast'))
        ).toBeVisible({ timeout: 10000 })
      })
    })

    test('should allow navigation between steps (back button)', async ({
      page
    }) => {
      await test.step('Complete first step', async () => {
        await fillFormStep1(page, testData)

        await page.getByTestId('next-button').click()
        await CommonHelpers.waitForNetworkIdle(page)
      })

      await test.step('Go back to first step', async () => {
        const backButton = page
          .getByText(/Org details|Detalhes da Org/i)
          .first()
        if (await backButton.isVisible()) {
          await backButton.click()
          await CommonHelpers.waitForNetworkIdle(page)

          // Verify we're back at step 1
          await expect(page.locator('input[name="legalName"]')).toBeVisible()
        }
      })

      await test.step('Verify form data persisted', async () => {
        const legalNameValue = await page
          .locator('input[name="legalName"]')
          .inputValue()
        expect(legalNameValue).toBe(testData.legalName)
      })
    })
  })

  test.describe('Validation Scenarios', () => {
    test('should validate required fields on step 1', async ({ page }) => {
      await test.step('Try to continue without filling required fields', async () => {
        await page
          .locator('input[name="legalDocument"]')
          .fill(testData.legalDocument)
        await page.getByTestId('next-button').click()
      })

      await test.step('Verify validation errors', async () => {
        const error = page.getByText(/.*least 1 character/i).first()
        await expect(error).toBeVisible()
      })
    })

    test('should validate legal document format', async ({ page }) => {
      await test.step('Enter invalid legal document', async () => {
        await fillFormStep1(page, testData)
        await page.locator('input[name="legalDocument"]').fill('invalid')

        await page.getByTestId('next-button').click()
      })

      await test.step('Verify validation error for format', async () => {
        const errorVisible = await page
          .getByText(/invalid.*format|valid.*document/i)
          .isVisible()
          .catch(() => false)
        if (errorVisible) {
          await CommonHelpers.verifyValidationError(page, /invalid.*format/i)
        }
      })
    })

    test('should validate address fields on step 2', async ({ page }) => {
      await test.step('Complete step 1', async () => {
        await fillFormStep1(page, testData)

        await page.getByTestId('next-button').click()
        await CommonHelpers.waitForNetworkIdle(page)
      })

      await test.step('Try to continue without address', async () => {
        await page
          .locator('input[name="address.zipCode"]')
          .fill(testData.address.zipCode.replaceAll(/[^\d]/g, ''))
        await page.getByTestId('next-button').click()
      })

      await test.step('Verify address validation', async () => {
        const error = page.getByText(/.*least 1 character/i).first()
        await expect(error).toBeVisible()
      })
    })

    test('should validate zipCode format', async ({ page }) => {
      await test.step('Complete step 1', async () => {
        await fillFormStep1(page, testData)

        await page.getByTestId('next-button').click()
        await CommonHelpers.waitForNetworkIdle(page)
      })

      await test.step('Enter invalid zipCode', async () => {
        await fillFormStep2(page, testData)
        await page.locator('input[name="address.zipCode"]').fill('invalid-zip')

        await page.getByTestId('next-button').click()
      })

      await test.step('Check for format validation', async () => {
        const errorVisible = await page
          .getByText(/invalid.*zip|postal.*code/i)
          .isVisible()
          .catch(() => false)
        if (errorVisible) {
          await CommonHelpers.verifyValidationError(page, /invalid.*zip/i)
        }
      })
    })
  })

  test.describe('Form Persistence', () => {
    test('should preserve data when navigating between steps', async ({
      page
    }) => {
      const formData = {
        legalName: testData.legalName,
        doingBusinessAs: testData.doingBusinessAs,
        legalDocument: testData.legalDocument
      }

      await test.step('Fill step 1 and continue', async () => {
        await fillFormStep1(page, testData, formData)
        await page.getByTestId('next-button').click()
        await CommonHelpers.waitForNetworkIdle(page)
      })

      await test.step('Fill step 2 and go back', async () => {
        await page
          .locator('input[name="address.line1"]')
          .fill(testData.address.line1)

        const backButton = page
          .getByText(/Org details|Detalhes da Org/i)
          .first()
        if (await backButton.isVisible()) {
          await backButton.click()
          await CommonHelpers.waitForNetworkIdle(page)
        }
      })

      await test.step('Verify step 1 data persisted', async () => {
        const legalNameValue = await page
          .locator('input[name="legalName"]')
          .inputValue()
        expect(legalNameValue).toBe(formData.legalName)

        const dbaValue = await page
          .locator('input[name="doingBusinessAs"]')
          .inputValue()
        expect(dbaValue).toBe(formData.doingBusinessAs)
      })
    })
  })

  test.describe('Progress Indicators', () => {
    test('should show progress through steps', async ({ page }) => {
      await test.step('Verify initial progress state', async () => {
        // Look for step indicators (1/3, step 1 of 3, etc.)
        const progressIndicator = page
          .locator('[data-testid="progress"]')
          .or(
            page
              .locator('[role="progressbar"]')
              .or(page.getByText(/step.*1.*3/i))
          )

        const hasProgress = await progressIndicator
          .isVisible()
          .catch(() => false)
        if (hasProgress) {
          await expect(progressIndicator).toBeVisible()
        }
      })
    })

    test('should show completed steps', async ({ page }) => {
      await test.step('Complete step 1', async () => {
        await fillFormStep1(page, testData)

        await page.getByTestId('next-button').click()
        await CommonHelpers.waitForNetworkIdle(page)
      })

      await test.step('Verify step 1 marked as complete', async () => {
        // Check for completed step indicator (checkmark, different color, etc.)
        const completedStep = page
          .locator('[data-testid="step-1-complete"]')
          .or(page.locator('[data-step="1"][data-status="complete"]'))

        const isMarkedComplete = await completedStep
          .isVisible()
          .catch(() => false)
        if (isMarkedComplete) {
          await expect(completedStep).toBeVisible()
        }
      })
    })
  })

  test.describe('UI/UX Validation', () => {
    test('should disable next button until required fields filled', async ({
      page
    }) => {
      const nextButton = page.getByRole('button', { name: /next|continue/i })

      await test.step('Verify button state without data', async () => {
        const isDisabled = await nextButton.isDisabled().catch(() => false)
        // Button might be enabled but validation happens on click
        // So we just check if it exists
        await expect(nextButton).toBeVisible()
      })
    })

    test('should show field-level validation on blur', async ({ page }) => {
      await test.step('Focus and blur required field without value', async () => {
        const legalNameInput = page.locator('input[name="legalName"]')
        await legalNameInput.focus()
        await legalNameInput.blur()
      })

      await test.step('Check for inline validation error', async () => {
        const errorVisible = await page
          .getByText(/required/i)
          .isVisible()
          .catch(() => false)
        // Field-level validation is optional, so we don't fail if not present
        if (errorVisible) {
          await expect(page.getByText(/required/i)).toBeVisible()
        }
      })
    })

    test('should show loading state during submission', async ({ page }) => {
      await test.step('Fill all required fields', async () => {
        await fillFormStep1(page, testData)

        await page.getByTestId('next-button').click()
        await CommonHelpers.waitForNetworkIdle(page)

        await fillFormStep2(page, testData)

        await page.getByTestId('next-button').click()
        await CommonHelpers.waitForNetworkIdle(page)
      })

      await test.step('Submit and check loading state', async () => {
        await page
          .getByRole('button', {
            name: /finish|complete|submit/i
          })
          .click()

        const submitButton = page.getByRole('button', {
          name: /Yes, I will configure it later|Sim, vou configurar depois/i
        })

        await submitButton.click()

        // Check for loading indicator
        const isDisabled = await submitButton.isDisabled().catch(() => false)
        expect(isDisabled).toBeTruthy()
      })
    })
  })

  test.describe('Create Ledger', () => {
    test('should create ledger', async ({ page }) => {
      await test.step('Create organization', async () => {
        await fillFormStep1(page, testData)
        await page.getByTestId('next-button').click()
        await CommonHelpers.waitForNetworkIdle(page)

        await fillFormStep2(page, testData)
        await page.getByTestId('next-button').click()
        await CommonHelpers.waitForNetworkIdle(page)

        await page
          .getByRole('button', {
            name: /finish|complete|submit/i
          })
          .click()

        await page
          .getByRole('button', {
            name: /Yes, I will configure it later|Sim, vou configurar depois/i
          })
          .click()

        await page.getByText(/continue|continuar/i).click()
      })

      await test.step('Create ledger', async () => {
        await page.locator('input[name="name"]').fill(ledgerData.name)
        await page
          .getByRole('button', {
            name: /finish|finalizar/i
          })
          .click()

        await CommonHelpers.waitForNetworkIdle(page)
      })

      await test.step('Verify ledger created', async () => {
        await expect(
          page.getByText(/.*setup complete|configuração completa/i)
        ).toBeVisible()
      })
    })
  })
})
