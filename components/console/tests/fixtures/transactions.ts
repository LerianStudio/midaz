/**
 * Test transaction fixtures for E2E tests
 * Based on MidazCreateTransactionDto schema
 */

import { ASSETS } from './assets'
import { ACCOUNTS } from './accounts'

/**
 * Simple transaction form data for UI testing
 */
export const SIMPLE_TRANSACTION_FORM_DATA = {
  E2E_BRL_DEPOSIT: {
    description: 'E2E Test Transaction',
    asset: ASSETS.BRL.code,
    value: '100',
    sourceAccount: `@external/${ASSETS.BRL.code}`,
    destinationAccount: ACCOUNTS.BRL_ACCOUNT.alias
  },
  E2E_BRL_DEPOSIT_WITH_METADATA: {
    description: 'E2E Test Transaction with Metadata',
    asset: ASSETS.BRL.code,
    value: '150',
    sourceAccount: `@external/${ASSETS.BRL.code}`,
    destinationAccount: ACCOUNTS.BRL_ACCOUNT.alias,
    metadata: {
      category: 'deposit',
      reference: 'TEST-REF-001'
    }
  }
} as const

/**
 * Complex transaction form data for UI testing
 */
export const COMPLEX_TRANSACTION_FORM_DATA = {
  E2E_MULTI_ACCOUNT: {
    description: 'E2E Complex Transaction - Multiple BRL Accounts',
    asset: ASSETS.BRL.code,
    value: '300',
    sourceAccounts: [
      ACCOUNTS.BRL_ACCOUNT.alias,
      ACCOUNTS.BRL_ACCOUNT_2.alias,
      ACCOUNTS.BRL_ACCOUNT_3.alias
    ],
    destinationAccounts: [
      ACCOUNTS.BRL_ACCOUNT_4.alias,
      `@external/${ASSETS.BRL.code}`
    ]
  }
} as const

export const TRANSACTIONS = {
  BRL_DEPOSIT: {
    chartOfAccountsGroupName: 'FUNDING',
    description: `Deposit 100 ${ASSETS.BRL.code} from external source`,
    send: {
      asset: ASSETS.BRL.code,
      value: '100',
      source: {
        from: [
          {
            accountAlias: `@external/${ASSETS.BRL.code}`,
            amount: {
              value: '100',
              asset: ASSETS.BRL.code
            },
            chartOfAccounts: 'FUNDING_DEBIT',
            description: `External ${ASSETS.BRL.code} source`
          }
        ]
      },
      distribute: {
        to: [
          {
            accountAlias: ACCOUNTS.BRL_ACCOUNT.alias,
            amount: {
              value: '100',
              asset: ASSETS.BRL.code
            },
            chartOfAccounts: 'FUNDING_CREDIT',
            description: `Credit to ${ASSETS.BRL.code} account`
          }
        ]
      }
    }
  },
  BTC_DEPOSIT: {
    chartOfAccountsGroupName: 'FUNDING',
    description: `Deposit 0.001 ${ASSETS.BTC.code} from external source`,
    send: {
      asset: ASSETS.BTC.code,
      value: '0.001',
      source: {
        from: [
          {
            accountAlias: `@external/${ASSETS.BTC.code}`,
            amount: {
              value: '0.001',
              asset: ASSETS.BTC.code
            },
            chartOfAccounts: 'FUNDING_DEBIT',
            description: `External ${ASSETS.BTC.code} source`
          }
        ]
      },
      distribute: {
        to: [
          {
            accountAlias: ACCOUNTS.BTC_ACCOUNT.alias,
            amount: {
              value: '0.001',
              asset: ASSETS.BTC.code
            },
            chartOfAccounts: 'FUNDING_CREDIT',
            description: `Credit to ${ASSETS.BTC.code} account`
          }
        ]
      }
    }
  }
} as const

export type TransactionFixture =
  (typeof TRANSACTIONS)[keyof typeof TRANSACTIONS]
