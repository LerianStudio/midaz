import { MetadataDto } from './metadata-dto'
import { StatusDto } from './status-dto'

export type CreateTransactionSourceDto = {
  account: string
  asset: string
  value: number
  description?: string
  chartOfAccounts?: string
  metadata: MetadataDto
}

export type CreateTransactionDto = {
  description?: string
  chartOfAccountsGroupName?: string
  value: number
  asset: string
  source: CreateTransactionSourceDto[]
  destination: CreateTransactionSourceDto[]
  metadata: MetadataDto
}

export type UpdateTransactionDto = {
  description?: string
  metadata?: Record<string, unknown>
}

export type TransactionOperationDto = CreateTransactionSourceDto & {
  accountAlias?: string
}

export type TransactionResponseDto = {
  id: string
  ledgerId: string
  organizationId: string
  description?: string
  chartOfAccountsGroupName?: string
  status: StatusDto
  value: number
  asset: string
  source: TransactionOperationDto[]
  destination: TransactionOperationDto[]
  metadata: MetadataDto
  createdAt: string
  updatedAt?: string
  deletedAt?: string
}
