import { test, expect } from '@playwright/test'

test('should redirect to home page', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByTestId('title')).toBeVisible()
})
