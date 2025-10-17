import { chromium, FullConfig } from '@playwright/test'
import path from 'path'
import fs from 'fs'
import {
  MIDAZ_CONSOLE_URL,
  MIDAZ_PASSWORD,
  MIDAZ_USERNAME
} from '../fixtures/config'

async function globalSetup(config: FullConfig) {
  // Validate required environment variables

  if (!MIDAZ_CONSOLE_URL) {
    throw new Error(
      'Missing MIDAZ_CONSOLE_URL. Please check .env.playwright configuration.'
    )
  }

  console.log('🔧 Global Setup: Starting authentication...')
  console.log(`📍 Target URL: ${MIDAZ_CONSOLE_URL}`)

  const browser = await chromium.launch()
  const page = await browser.newPage()

  try {
    // Navigate to login page
    await page.goto(MIDAZ_CONSOLE_URL, {
      waitUntil: 'domcontentloaded',
      timeout: 60000
    })

    // Wait for successful navigation (could be redirect to home/dashboard)
    await page.waitForURL(/.*/, { timeout: 10000 }) // Wait for any URL change
    await page.waitForLoadState('domcontentloaded')
    console.log(`✅ Authentication successful, redirected to: ${page.url()}`)

    // Ensure storage directory exists
    const storagePath = path.join(process.cwd(), 'tests/storage')
    if (!fs.existsSync(storagePath)) {
      fs.mkdirSync(storagePath, { recursive: true })
      console.log('✅ Created storage directory')
    }

    // Save authenticated state
    const storageFile = path.join(storagePath, 'data.json')
    await page.context().storageState({ path: storageFile })
    console.log(`✅ Storage state saved to: ${storageFile}`)

    console.log('🎉 Global Setup: Authentication complete!')
  } catch (error) {
    console.error('❌ Global Setup Failed:', error)
    throw error
  } finally {
    await browser.close()
  }
}

export default globalSetup
