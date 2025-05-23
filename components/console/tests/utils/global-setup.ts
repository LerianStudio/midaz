import { chromium } from '@playwright/test'
import {
  MIDAZ_CONSOLE_URL,
  MIDAZ_PASSWORD,
  MIDAZ_USERNAME
} from '../fixtures/config'

async function globalSetup() {
  const browser = await chromium.launch()
  const page = await browser.newPage()

  await page.goto(MIDAZ_CONSOLE_URL + '/signin')
  await page.waitForLoadState('networkidle')

  await page.fill('input[name="username"]', MIDAZ_USERNAME!)
  await page.fill('input[name="password"]', MIDAZ_PASSWORD!)
  await page.click('button[type="submit"]')
  await page.waitForURL(MIDAZ_CONSOLE_URL)

  await page.context().storageState({ path: 'tests/storage/data.json' })

  await browser.close()
}

export default globalSetup
