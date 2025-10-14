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
  E2E_SIMPLE_TRANSFER: {
    description: 'E2E Complex Transaction - Simple 1:1 Transfer',
    asset: ASSETS.BRL.code,
    value: '50',
    sourceAccounts: [`@external/${ASSETS.BRL.code}`],
    destinationAccounts: [ACCOUNTS.BRL_ACCOUNT.alias]
  },
  E2E_ONE_TO_TWO_TRANSFER: {
    description: 'E2E Complex Transaction - 1:2 Transfer (Split)',
    asset: ASSETS.BRL.code,
    value: '100',
    sourceAccounts: [`@external/${ASSETS.BRL.code}`],
    destinationAccounts: [
      ACCOUNTS.BRL_ACCOUNT_2.alias,
      ACCOUNTS.BRL_ACCOUNT_3.alias
    ],
    destinationAmounts: ['50', '50'] // Split equally
  },
  E2E_TWO_TO_ONE_TRANSFER: {
    description: 'E2E Complex Transaction - 2:1 Transfer (Merge)',
    asset: ASSETS.BRL.code,
    value: '2',
    sourceAccounts: [ACCOUNTS.BRL_ACCOUNT.alias, ACCOUNTS.BRL_ACCOUNT_2.alias],
    destinationAccounts: [ACCOUNTS.BRL_ACCOUNT_3.alias],
    sourceAmounts: ['1', '1'] // 1 BRL from each source
  },
  E2E_TWO_TO_TWO_TRANSFER: {
    description: 'E2E Complex Transaction - 2:2 Transfer (Multi-split)',
    asset: ASSETS.BRL.code,
    value: '2',
    sourceAccounts: [ACCOUNTS.BRL_ACCOUNT.alias, ACCOUNTS.BRL_ACCOUNT_2.alias],
    destinationAccounts: [
      ACCOUNTS.BRL_ACCOUNT_3.alias,
      ACCOUNTS.BRL_ACCOUNT_4.alias
    ],
    sourceAmounts: ['1', '1'], // 1 BRL from each source
    destinationAmounts: ['1', '1'] // 1 BRL to each destination
  },
  E2E_ONE_TO_TWO_INVALID_AMOUNTS: {
    description: 'E2E Complex Transaction - 1:2 Invalid Split',
    asset: ASSETS.BRL.code,
    value: '100',
    sourceAccounts: [`@external/${ASSETS.BRL.code}`],
    destinationAccounts: [
      ACCOUNTS.BRL_ACCOUNT_2.alias,
      ACCOUNTS.BRL_ACCOUNT_3.alias
    ],
    destinationAmounts: ['50', '30'] // Only 80 BRL distributed, missing 20
  },
  E2E_DUPLICATE_SOURCE_ACCOUNT: {
    description: 'E2E Complex Transaction - Duplicate Source Account',
    asset: ASSETS.BRL.code,
    value: '50',
    sourceAccounts: [
      ACCOUNTS.BRL_ACCOUNT_5.alias,
      ACCOUNTS.BRL_ACCOUNT_5.alias
    ], // Same account twice
    destinationAccounts: [`@external/${ASSETS.BRL.code}`]
  },
  E2E_INSUFFICIENT_FUNDS: {
    description: 'E2E Complex Transaction - Insufficient Funds',
    asset: ASSETS.BRL.code,
    value: '900',
    sourceAccounts: [ACCOUNTS.BRL_ACCOUNT_5.alias], // Only has 100 BRL
    destinationAccounts: [`@external/${ASSETS.BRL.code}`]
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
  },
  BRL_DEPOSIT_2: {
    chartOfAccountsGroupName: 'FUNDING',
    description: `Deposit 100 ${ASSETS.BRL.code} to account 2`,
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
            accountAlias: ACCOUNTS.BRL_ACCOUNT_2.alias,
            amount: {
              value: '100',
              asset: ASSETS.BRL.code
            },
            chartOfAccounts: 'FUNDING_CREDIT',
            description: `Credit to ${ASSETS.BRL.code} account 2`
          }
        ]
      }
    }
  },
  BRL_DEPOSIT_3: {
    chartOfAccountsGroupName: 'FUNDING',
    description: `Deposit 100 ${ASSETS.BRL.code} to account 3`,
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
            accountAlias: ACCOUNTS.BRL_ACCOUNT_3.alias,
            amount: {
              value: '100',
              asset: ASSETS.BRL.code
            },
            chartOfAccounts: 'FUNDING_CREDIT',
            description: `Credit to ${ASSETS.BRL.code} account 3`
          }
        ]
      }
    }
  },
  BRL_DEPOSIT_4: {
    chartOfAccountsGroupName: 'FUNDING',
    description: `Deposit 100 ${ASSETS.BRL.code} to account 4`,
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
            accountAlias: ACCOUNTS.BRL_ACCOUNT_4.alias,
            amount: {
              value: '100',
              asset: ASSETS.BRL.code
            },
            chartOfAccounts: 'FUNDING_CREDIT',
            description: `Credit to ${ASSETS.BRL.code} account 4`
          }
        ]
      }
    }
  },
  BRL_DEPOSIT_5: {
    chartOfAccountsGroupName: 'FUNDING',
    description: `Deposit 100 ${ASSETS.BRL.code} to account 5`,
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
            accountAlias: ACCOUNTS.BRL_ACCOUNT_5.alias,
            amount: {
              value: '100',
              asset: ASSETS.BRL.code
            },
            chartOfAccounts: 'FUNDING_CREDIT',
            description: `Credit to ${ASSETS.BRL.code} account 5`
          }
        ]
      }
    }
  },
  BTC_DEPOSIT_2: {
    chartOfAccountsGroupName: 'FUNDING',
    description: `Deposit 0.001 ${ASSETS.BTC.code} to account 2`,
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
            accountAlias: ACCOUNTS.BTC_ACCOUNT_2.alias,
            amount: {
              value: '0.001',
              asset: ASSETS.BTC.code
            },
            chartOfAccounts: 'FUNDING_CREDIT',
            description: `Credit to ${ASSETS.BTC.code} account 2`
          }
        ]
      }
    }
  },
  BTC_DEPOSIT_3: {
    chartOfAccountsGroupName: 'FUNDING',
    description: `Deposit 0.001 ${ASSETS.BTC.code} to account 3`,
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
            accountAlias: ACCOUNTS.BTC_ACCOUNT_3.alias,
            amount: {
              value: '0.001',
              asset: ASSETS.BTC.code
            },
            chartOfAccounts: 'FUNDING_CREDIT',
            description: `Credit to ${ASSETS.BTC.code} account 3`
          }
        ]
      }
    }
  },
  BTC_DEPOSIT_4: {
    chartOfAccountsGroupName: 'FUNDING',
    description: `Deposit 0.001 ${ASSETS.BTC.code} to account 4`,
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
            accountAlias: ACCOUNTS.BTC_ACCOUNT_4.alias,
            amount: {
              value: '0.001',
              asset: ASSETS.BTC.code
            },
            chartOfAccounts: 'FUNDING_CREDIT',
            description: `Credit to ${ASSETS.BTC.code} account 4`
          }
        ]
      }
    }
  }
} as const

export type TransactionFixture =
  (typeof TRANSACTIONS)[keyof typeof TRANSACTIONS]
