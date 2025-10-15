import { defineConfig, devices } from '@playwright/test'
import dotenv from 'dotenv'
import path from 'path'

// Load test environment variables
dotenv.config({ path: path.resolve(__dirname, '.env.playwright') })

const CONSOLE_HOST = process.env.MIDAZ_CONSOLE_HOST || 'localhost'
const CONSOLE_PORT = process.env.MIDAZ_CONSOLE_PORT || '8081'
const BASE_URL = `http://${CONSOLE_HOST}:${CONSOLE_PORT}`

export default defineConfig({
  testDir: './tests/e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  timeout: 120 * 1000, // 120 seconds per test (increased for slow server startup)
  expect: {
    timeout: 30000 // 300 seconds for assertions (increased for data loading)
  },
  reporter: process.env.CI
    ? [['html'], ['json', { outputFile: 'test-results.json' }], ['github']]
    : [['html'], ['list']],
  globalSetup: './tests/utils/global-setup.ts',
  use: {
    baseURL: BASE_URL,
    storageState: 'tests/storage/data.json',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    video: 'on',
    actionTimeout: 10 * 1000 // 10 seconds for actions,
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] }
    },
    // {
    //   name: 'firefox',
    //   use: { ...devices['Desktop Firefox'] }
    // },

    // {
    //   name: 'webkit',
    //   use: { ...devices['Desktop Safari'] }
    // }
  ]

  // webServer is disabled when testing against the Docker container
  // If you need to run tests against a local dev server, uncomment this:
  // webServer: {
  //   command: 'npm run dev',
  //   port: parseInt(CONSOLE_PORT, 10),
  //   reuseExistingServer: !process.env.CI,
  //   timeout: 120 * 1000, // 2 minutes
  //   stdout: 'pipe',
  //   stderr: 'pipe'
  // }
})
