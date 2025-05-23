import { StatusDto } from './status.dto'

type CreateTransactionSourceDto = {
  account: string
  asset: string
  value: number
  share?: {
    percentage: number
    percentageOfPercentage: number
  }
  description?: string
  chartOfAccounts?: string
  metadata: Record<string, any>
}

export type CreateTransactionDto = {
  description?: string
  chartOfAccountsGroupName?: string
  value: number
  asset: string
  source: CreateTransactionSourceDto[]
  destination: CreateTransactionSourceDto[]
  metadata: Record<string, any>
}

export type OperationDto = {
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
  status: StatusDto
  accountId: string
  accountAlias: string
  organizationId: string
  ledgerId: string
  portfolioId?: string
  createdAt?: string
  updatedAt?: string
  deletedAt?: string
  metadata: Record<string, unknown>
}

export type TransactionResponseDto = {
  id: string
  description?: string
  template: string
  status: StatusDto
  amount: number
  amountScale: number
  assetCode: string
  chartOfAccountsGroupName: string
  source: string[]
  destination: string[]
  ledgerId: string
  organizationId: string
  operations: OperationDto[]
  metadata: Record<string, unknown>
  createdAt?: string
  updatedAt?: string
  deletedAt?: string
}
