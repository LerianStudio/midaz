export type TransactionResponse = TransactionType[]

export interface TransactionType {
  id: string
  description: string
  template: string
  status: {
    code: string
    description: string
  }
  amount: number
  amountScale: number
  assetCode: string
  chartOfAccountsGroupName: string
  source: string[]
  destination: string[]
  ledgerId: string
  organizationId: string
  createdAt: string
  updatedAt: string
  deletedAt: string | null
  metadata: Record<string, any> | null
  operations: Operation[]
}

export interface Operation {
  id: string
  transactionId: string
  description: string
  type: 'DEBIT' | 'CREDIT' | ''
  assetCode: string
  chartOfAccounts: string
  amount: {
    amount: number
    scale: number
  }
  balance: {
    available: number
    onHold: number
    scale: number
  }
  balanceAfter: {
    available: number
    onHold: number
    scale: number
  }
  status: {
    code: string
    description: string | null
  }
  accountId: string
  accountAlias: string
  portfolioId: string | null
  organizationId: string
  ledgerId: string
  createdAt: string
  updatedAt: string
  deletedAt: string | null
  metadata: Record<string, any> | null
}
