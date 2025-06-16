import { MidazMetadataDto } from './midaz-metadata-dto'
import { MidazStatusDto } from './midaz-status-dto'

type MidazBalanceDto = {
  available: number
  onHold: number
  scale: number
}

type MidazCreateSourceDto = {
  accountAlias: string
  amount: {
    value: string
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
    value: string
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
  amount: {
    value: string
  }
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
  amount: string
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
