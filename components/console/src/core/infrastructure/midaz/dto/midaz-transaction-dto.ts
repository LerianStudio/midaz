import { MidazMetadataDto } from './midaz-metadata-dto'
import { MidazStatusDto } from './midaz-status-dto'

type MidazAmountDto = {
  amount: number
  scale: number
}

type MidazBalanceDto = {
  available: number
  onHold: number
  scale: number
}

type MidazCreateSourceDto = {
  account: string
  amount: {
    value: number
    scale: number
    asset: string
  }
  share?: {
    percentage: number
    percentageOfPercentage: number
  }
  chartOfAccounts?: string
  description?: string
  metadata?: MidazMetadataDto
}

export type MidazCreateTransactionDto = {
  description?: string
  chartOfAccountsGroupName?: string
  send: {
    asset: string
    value: number
    scale: number
    source: {
      from: MidazCreateSourceDto[]
    }
    distribute: {
      to: MidazCreateSourceDto[]
    }
  }
  metadata?: MidazMetadataDto
}

export type MidazUpdateTransactionDto = {
  description?: string
  metadata?: MidazMetadataDto
}

export type OperationDto = {
  id: string
  transactionId: string
  organizationId: string
  ledgerId: string
  description: string
  type: string
  assetCode: string
  chartOfAccounts?: string
  amount: MidazAmountDto
  balance: MidazBalanceDto
  balanceAfter: MidazBalanceDto
  status: MidazStatusDto
  accountId: string
  accountAlias: string
  portfolioId?: string
  createdAt: string
  updatedAt?: string
  deletedAt?: string
  metadata?: MidazMetadataDto
}

export type MidazTransactionDto = {
  id: string
  parentTransactionId?: string
  ledgerId: string
  organizationId: string
  description?: string
  type: string
  template: string
  status: MidazStatusDto
  amount: number
  amountScale: number
  assetCode: string
  chartOfAccountsGroupName?: string
  source: string[]
  destination: string[]
  operations: OperationDto[]
  metadata?: MidazMetadataDto
  createdAt: string
  updatedAt?: string
  deletedAt?: string
}
