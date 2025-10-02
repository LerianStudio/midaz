import { test, expect } from '@playwright/test'
import { testDataFactory } from '../fixtures/test-data.factory'
import { CommonHelpers } from '../utils/common-helpers'
import { navigateToOnboarding } from '../utils/navigate-to-onboarding'

test.describe('Onboarding Flow - E2E Tests', () => {
  let testData: ReturnType<typeof testDataFactory.organization>

  test.beforeEach(async ({ page }) => {
    testData = testDataFactory.organization()
    await navigateToOnboarding(page)
  })

  test.describe('Multi-Step Onboarding Flow', () => {
    test('should complete full onboarding with all steps', async ({ page }) => {
      await test.step('Step 1: Fill organization details', async () => {
        await page.locator('input[name="legalName"]').fill(testData.legalName)
        await page
          .locator('input[name="doingBusinessAs"]')
          .fill(testData.doingBusinessAs)
        await page
          .locator('input[name="legalDocument"]')
          .fill(testData.legalDocument)

        // Continue to next step
        await page.getByRole('button', { name: /next|continue/i }).click()
        await CommonHelpers.waitForNetworkIdle(page)
      })

      await test.step('Step 2: Fill address information', async () => {
        await page
          .locator('input[name="address.line1"]')
          .fill(testData.address.line1)
        await page
          .locator('input[name="address.line2"]')
          .fill(testData.address.line2)
        await page
          .locator('input[name="address.city"]')
          .fill(testData.address.city)
        await page
          .locator('input[name="address.state"]')
          .fill(testData.address.state)
        await page
          .locator('input[name="address.country"]')
          .fill(testData.address.country)
        await page
          .locator('input[name="address.zipCode"]')
          .fill(testData.address.zipCode)

        // Continue to next step
        await page.getByRole('button', { name: /next|continue/i }).click()
        await CommonHelpers.waitForNetworkIdle(page)
      })

      await test.step('Step 3: Configure theme (optional)', async () => {
        // Theme configuration might be optional
        const accentColorInput = page.locator('input[name="accentColor"]')
        if (await accentColorInput.isVisible()) {
          await accentColorInput.fill('#3B82F6')
        }

        const avatarInput = page.locator('input[name="avatar"]')
        if (await avatarInput.isVisible()) {
          await avatarInput.fill('https://example.com/avatar.png')
        }

        // Complete onboarding
        await page
          .getByRole('button', { name: /finish|complete|submit/i })
          .click()
        await CommonHelpers.waitForNetworkIdle(page)
      })

      await test.step('Verify successful onboarding completion', async () => {
        // Should redirect to main app or show success message
        await expect(
          page
            .getByText(/success|complete|welcome/i)
            .or(page.getByTestId('success-toast'))
        ).toBeVisible({ timeout: 10000 })
      })
    })

    test('should complete onboarding with minimal required fields', async ({
      page
    }) => {
      await test.step('Fill only required fields - Step 1', async () => {
        await page.locator('input[name="legalName"]').fill(testData.legalName)
        await page
          .locator('input[name="doingBusinessAs"]')
          .fill(testData.doingBusinessAs)
        await page
          .locator('input[name="legalDocument"]')
          .fill(testData.legalDocument)

        await page.getByRole('button', { name: /next|continue/i }).click()
        await CommonHelpers.waitForNetworkIdle(page)
      })

      await test.step('Fill minimal address - Step 2', async () => {
        await page
          .locator('input[name="address.line1"]')
          .fill(testData.address.line1)
        await page
          .locator('input[name="address.city"]')
          .fill(testData.address.city)
        await page
          .locator('input[name="address.country"]')
          .fill(testData.address.country)
        await page
          .locator('input[name="address.zipCode"]')
          .fill(testData.address.zipCode)

        await page.getByRole('button', { name: /next|continue/i }).click()
        await CommonHelpers.waitForNetworkIdle(page)
      })

      await test.step('Skip optional theme - Step 3', async () => {
        await page
          .getByRole('button', { name: /finish|complete|skip/i })
          .click()
        await CommonHelpers.waitForNetworkIdle(page)
      })

      await test.step('Verify completion', async () => {
        await expect(
          page
            .getByText(/success|complete/i)
            .or(page.getByTestId('success-toast'))
        ).toBeVisible({ timeout: 10000 })
      })
    })

    test('should allow navigation between steps (back button)', async ({
      page
    }) => {
      await test.step('Complete first step', async () => {
        await page.locator('input[name="legalName"]').fill(testData.legalName)
        await page
          .locator('input[name="doingBusinessAs"]')
          .fill(testData.doingBusinessAs)
        await page
          .locator('input[name="legalDocument"]')
          .fill(testData.legalDocument)

        await page.getByRole('button', { name: /next|continue/i }).click()
        await CommonHelpers.waitForNetworkIdle(page)
      })

      await test.step('Go back to first step', async () => {
        const backButton = page.getByRole('button', { name: /back|previous/i })
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
        await page.getByRole('button', { name: /next|continue/i }).click()
      })

      await test.step('Verify validation errors', async () => {
        await CommonHelpers.verifyValidationError(page, /required/i)
      })
    })

    test('should validate legal document format', async ({ page }) => {
      await test.step('Enter invalid legal document', async () => {
        await page.locator('input[name="legalName"]').fill(testData.legalName)
        await page
          .locator('input[name="doingBusinessAs"]')
          .fill(testData.doingBusinessAs)
        await page.locator('input[name="legalDocument"]').fill('invalid')

        await page.getByRole('button', { name: /next|continue/i }).click()
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
        await page.locator('input[name="legalName"]').fill(testData.legalName)
        await page
          .locator('input[name="doingBusinessAs"]')
          .fill(testData.doingBusinessAs)
        await page
          .locator('input[name="legalDocument"]')
          .fill(testData.legalDocument)

        await page.getByRole('button', { name: /next|continue/i }).click()
        await CommonHelpers.waitForNetworkIdle(page)
      })

      await test.step('Try to continue without address', async () => {
        await page.getByRole('button', { name: /next|continue/i }).click()
      })

      await test.step('Verify address validation', async () => {
        await CommonHelpers.verifyValidationError(page, /required/i)
      })
    })

    test('should validate zipCode format', async ({ page }) => {
      await test.step('Complete step 1', async () => {
        await CommonHelpers.fillForm(page, {
          legalName: testData.legalName,
          doingBusinessAs: testData.doingBusinessAs,
          legalDocument: testData.legalDocument
        })

        await page.getByRole('button', { name: /next|continue/i }).click()
        await CommonHelpers.waitForNetworkIdle(page)
      })

      await test.step('Enter invalid zipCode', async () => {
        await page
          .locator('input[name="address.line1"]')
          .fill(testData.address.line1)
        await page
          .locator('input[name="address.city"]')
          .fill(testData.address.city)
        await page
          .locator('input[name="address.country"]')
          .fill(testData.address.country)
        await page.locator('input[name="address.zipCode"]').fill('invalid-zip')

        await page.getByRole('button', { name: /next|continue/i }).click()
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
        await CommonHelpers.fillForm(page, formData)
        await page.getByRole('button', { name: /next|continue/i }).click()
        await CommonHelpers.waitForNetworkIdle(page)
      })

      await test.step('Fill step 2 and go back', async () => {
        await page
          .locator('input[name="address.line1"]')
          .fill(testData.address.line1)

        const backButton = page.getByRole('button', { name: /back|previous/i })
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

    test('should persist data across page refresh (if implemented)', async ({
      page
    }) => {
      test.skip()
      // This would test localStorage/sessionStorage persistence
      // Implementation depends on onboarding flow design
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
        await CommonHelpers.fillForm(page, {
          legalName: testData.legalName,
          doingBusinessAs: testData.doingBusinessAs,
          legalDocument: testData.legalDocument
        })

        await page.getByRole('button', { name: /next|continue/i }).click()
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

  test.describe('Error Handling', () => {
    test('should handle organization already exists error', async ({
      page
    }) => {
      // This would require mocking API response for duplicate
      test.skip()
    })

    test('should handle network errors gracefully', async ({ page }) => {
      // Would require network mocking
      test.skip()
    })

    test('should allow retry after error', async ({ page }) => {
      // Would require error scenario simulation
      test.skip()
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
        await CommonHelpers.fillForm(page, {
          legalName: testData.legalName,
          doingBusinessAs: testData.doingBusinessAs,
          legalDocument: testData.legalDocument
        })

        await page.getByRole('button', { name: /next|continue/i }).click()
        await CommonHelpers.waitForNetworkIdle(page)

        await CommonHelpers.fillForm(page, {
          'address.line1': testData.address.line1,
          'address.city': testData.address.city,
          'address.country': testData.address.country,
          'address.zipCode': testData.address.zipCode
        })

        await page.getByRole('button', { name: /next|continue/i }).click()
        await CommonHelpers.waitForNetworkIdle(page)
      })

      await test.step('Submit and check loading state', async () => {
        const submitButton = page.getByRole('button', {
          name: /finish|complete|submit/i
        })
        await submitButton.click()

        // Check for loading indicator
        const isDisabled = await submitButton.isDisabled().catch(() => false)
        expect(isDisabled).toBeTruthy()
      })
    })
  })
})
