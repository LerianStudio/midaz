/**
 * Test asset fixtures for E2E tests
 */

export const ASSETS = {
  BRL: {
    name: 'Real',
    type: 'currency',
    code: 'BRL',
    status: {
      code: 'ACTIVE',
      description: 'Brazilian Real currency for E2E tests'
    }
  },
  BTC: {
    name: 'Bitcoin',
    type: 'crypto',
    code: 'BTC',
    status: {
      code: 'ACTIVE',
      description: 'Bitcoin cryptocurrency for E2E tests'
    }
  }
} as const

export type AssetFixture = (typeof ASSETS)[keyof typeof ASSETS]
