/**
 * Test account fixtures for E2E tests
 */

import { ASSETS } from './assets'

export const ACCOUNTS = {
  BRL_ACCOUNT: {
    name: 'BRL Account',
    assetCode: ASSETS.BRL.code,
    type: 'deposit',
    alias: 'brl-account-e2e',
    status: {
      code: 'ACTIVE',
      description: 'Brazilian Real account for E2E tests'
    }
  },
  BRL_ACCOUNT_2: {
    name: 'BRL Account 2',
    assetCode: ASSETS.BRL.code,
    type: 'deposit',
    alias: 'brl-account-2-e2e',
    status: {
      code: 'ACTIVE',
      description: 'Brazilian Real account 2 for E2E tests'
    }
  },
  BRL_ACCOUNT_3: {
    name: 'BRL Account 3',
    assetCode: ASSETS.BRL.code,
    type: 'deposit',
    alias: 'brl-account-3-e2e',
    status: {
      code: 'ACTIVE',
      description: 'Brazilian Real account 3 for E2E tests'
    }
  },
  BRL_ACCOUNT_4: {
    name: 'BRL Account 4',
    assetCode: ASSETS.BRL.code,
    type: 'deposit',
    alias: 'brl-account-4-e2e',
    status: {
      code: 'ACTIVE',
      description: 'Brazilian Real account 4 for E2E tests'
    }
  },
  BRL_ACCOUNT_5: {
    name: 'BRL Account 5',
    assetCode: ASSETS.BRL.code,
    type: 'deposit',
    alias: 'brl-account-5-e2e',
    status: {
      code: 'ACTIVE',
      description: 'Brazilian Real account 5 for E2E tests'
    }
  },
  BTC_ACCOUNT: {
    name: 'BTC Account',
    assetCode: ASSETS.BTC.code,
    type: 'deposit',
    alias: 'btc-account-e2e',
    status: {
      code: 'ACTIVE',
      description: 'Bitcoin account for E2E tests'
    }
  },
  BTC_ACCOUNT_2: {
    name: 'BTC Account 2',
    assetCode: ASSETS.BTC.code,
    type: 'deposit',
    alias: 'btc-account-2-e2e',
    status: {
      code: 'ACTIVE',
      description: 'Bitcoin account 2 for E2E tests'
    }
  },
  BTC_ACCOUNT_3: {
    name: 'BTC Account 3',
    assetCode: ASSETS.BTC.code,
    type: 'deposit',
    alias: 'btc-account-3-e2e',
    status: {
      code: 'ACTIVE',
      description: 'Bitcoin account 3 for E2E tests'
    }
  },
  BTC_ACCOUNT_4: {
    name: 'BTC Account 4',
    assetCode: ASSETS.BTC.code,
    type: 'deposit',
    alias: 'btc-account-4-e2e',
    status: {
      code: 'ACTIVE',
      description: 'Bitcoin account 4 for E2E tests'
    }
  }
} as const

export type AccountFixture = (typeof ACCOUNTS)[keyof typeof ACCOUNTS]
