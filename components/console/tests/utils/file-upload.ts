import * as path from 'path'
import { Page } from '@playwright/test'

/**
 * File Upload Helper
 * Utilities for handling file uploads in E2E tests
 */
export class FileUploadHelper {
  /**
   * Upload a test fixture image
   *
   * @param page - Playwright page object
   * @param selector - CSS selector or test ID for the file input element
   * @param fileName - Name of the file in tests/fixtures/images/ directory
   * @param options - Additional options for the upload operation
   * @param options.waitAfterUpload - Time to wait after upload (default: 500ms)
   *
   * @example
   * // Upload using CSS selector
   * await FileUploadHelper.uploadImage(page, '#avatar', 'test-avatar.png')
   *
   * @example
   * // Upload using test ID
   * await FileUploadHelper.uploadImage(page, '[data-testid="avatar-input"]', 'test-logo.jpg')
   *
   * @example
   * // Upload with custom wait time
   * await FileUploadHelper.uploadImage(page, '#avatar', 'test-avatar.png', { waitAfterUpload: 1000 })
   */
  static async uploadImage(
    page: Page,
    selector: string,
    fileName: string,
    options: {
      waitAfterUpload?: number
    } = {}
  ): Promise<void> {
    const { waitAfterUpload = 500 } = options

    const imagePath = path.join(__dirname, '../fixtures/images', fileName)
    const fileInput = page.locator(selector)

    // Verify the input exists
    const inputCount = await fileInput.count()
    if (inputCount === 0) {
      throw new Error(`File input with selector "${selector}" not found`)
    }

    // Upload the file
    await fileInput.setInputFiles(imagePath)

    // Wait for upload to process
    if (waitAfterUpload > 0) {
      await page.waitForTimeout(waitAfterUpload)
    }
  }

  /**
   * Upload a test fixture image by test ID
   * Convenience method that automatically adds the data-testid selector
   *
   * @param page - Playwright page object
   * @param testId - The data-testid attribute value
   * @param fileName - Name of the file in tests/fixtures/images/ directory
   * @param options - Additional options for the upload operation
   *
   * @example
   * await FileUploadHelper.uploadImageByTestId(page, 'avatar-input', 'test-avatar.png')
   */
  static async uploadImageByTestId(
    page: Page,
    testId: string,
    fileName: string,
    options: {
      waitAfterUpload?: number
    } = {}
  ): Promise<void> {
    await this.uploadImage(page, `[data-testid="${testId}"]`, fileName, options)
  }

  /**
   * Upload multiple files to an input
   *
   * @param page - Playwright page object
   * @param selector - CSS selector for the file input element
   * @param fileNames - Array of file names in tests/fixtures/images/ directory
   * @param options - Additional options for the upload operation
   *
   * @example
   * await FileUploadHelper.uploadMultipleFiles(page, '#documents', ['doc1.pdf', 'doc2.pdf'])
   */
  static async uploadMultipleFiles(
    page: Page,
    selector: string,
    fileNames: string[],
    options: {
      waitAfterUpload?: number
    } = {}
  ): Promise<void> {
    const { waitAfterUpload = 500 } = options

    const filePaths = fileNames.map((fileName) =>
      path.join(__dirname, '../fixtures/images', fileName)
    )

    const fileInput = page.locator(selector)

    // Verify the input exists
    const inputCount = await fileInput.count()
    if (inputCount === 0) {
      throw new Error(`File input with selector "${selector}" not found`)
    }

    // Upload the files
    await fileInput.setInputFiles(filePaths)

    // Wait for upload to process
    if (waitAfterUpload > 0) {
      await page.waitForTimeout(waitAfterUpload)
    }
  }

  /**
   * Clear a file input (remove selected files)
   *
   * @param page - Playwright page object
   * @param selector - CSS selector for the file input element
   *
   * @example
   * await FileUploadHelper.clearFileInput(page, '#avatar')
   */
  static async clearFileInput(page: Page, selector: string): Promise<void> {
    const fileInput = page.locator(selector)

    // Verify the input exists
    const inputCount = await fileInput.count()
    if (inputCount === 0) {
      throw new Error(`File input with selector "${selector}" not found`)
    }

    // Clear the input by setting empty array
    await fileInput.setInputFiles([])
  }

  /**
   * Upload a file if the input exists and is visible
   * Useful for optional file upload fields
   *
   * @param page - Playwright page object
   * @param selector - CSS selector for the file input element
   * @param fileName - Name of the file in tests/fixtures/images/ directory
   * @param options - Additional options for the upload operation
   * @returns boolean - true if upload was performed, false if input not found/visible
   *
   * @example
   * const uploaded = await FileUploadHelper.uploadImageIfExists(page, '#avatar', 'test-avatar.png')
   * if (uploaded) {
   *   console.log('Avatar uploaded successfully')
   * }
   */
  static async uploadImageIfExists(
    page: Page,
    selector: string,
    fileName: string,
    options: {
      waitAfterUpload?: number
      timeout?: number
    } = {}
  ): Promise<boolean> {
    const { timeout = 1000 } = options

    const fileInput = page.locator(selector)

    // Check if input exists and is visible
    const isVisible = await fileInput
      .isVisible({ timeout })
      .catch(() => false)

    if (!isVisible) {
      return false
    }

    // Upload the file
    await this.uploadImage(page, selector, fileName, options)
    return true
  }

  /**
   * Get the path to a test fixture image
   * Useful if you need the path for custom operations
   *
   * @param fileName - Name of the file in tests/fixtures/images/ directory
   * @returns Absolute path to the image file
   *
   * @example
   * const imagePath = FileUploadHelper.getImagePath('test-avatar.png')
   * console.log('Image located at:', imagePath)
   */
  static getImagePath(fileName: string): string {
    return path.join(__dirname, '../fixtures/images', fileName)
  }
}
