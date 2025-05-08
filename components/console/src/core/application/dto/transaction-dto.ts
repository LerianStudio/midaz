import { MetadataDto } from './metadata-dto'
import { StatusDto } from './status-dto'

export type CreateTransactionOperationDto = {
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
  source: CreateTransactionOperationDto[]
  destination: CreateTransactionOperationDto[]
  metadata: MetadataDto
}

export type UpdateTransactionDto = {
  description?: string
  metadata?: Record<string, unknown>
}

export type TransactionOperationDto = CreateTransactionOperationDto & {
  accountAlias?: string
}

export type TransactionDto = {
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
