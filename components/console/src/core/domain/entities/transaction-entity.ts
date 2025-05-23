import { StatusEntity } from './status-entity'

type TransactionSourceEntity = {
  account: string
  amount: {
    asset: string
    value: number
    scale: number
  }
  share?: {
    percentage: number
    percentageOfPercentage: number
  }
  chartOfAccounts?: string
  description?: string
  metadata: Record<string, any> | null
}

export type TransactionUpdateEntity = {
  description?: string
  metadata?: Record<string, unknown>
}

export type TransactionCreateEntity = {
  id?: string
  description?: string
  chartOfAccountsGroupName?: string
  send: {
    asset: string
    value: number
    scale: number
    source: {
      from: TransactionSourceEntity[]
    }
    distribute: {
      to: TransactionSourceEntity[]
    }
  }
  metadata: Record<string, any> | null
}

export type OperationEntity = {
  id: string
  transactionId: string
  description: string
  type: string
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
  status: StatusEntity
  accountId: string
  accountAlias: string
  portfolioId?: string
  organizationId: string
  ledgerId: string
  createdAt?: string
  updatedAt?: string
  deletedAt?: string
  metadata?: Record<string, unknown>
}

export type TransactionEntity = {
  id: string
  description: string
  template: string
  status: StatusEntity
  amount: number
  amountScale: number
  assetCode: string
  chartOfAccountsGroupName: string
  source: string[]
  destination: string[]
  ledgerId: string
  organizationId: string
  operations?: OperationEntity[]
  metadata?: Record<string, unknown>
  createdAt?: string
  updatedAt?: string
  deletedAt?: string
}
