import { MetadataDto } from './metadata-dto'
import { StatusDto } from './status-dto'

export type AmountDto = {
  value: number
  scale: number
}

export type CreateTransactionOperationDto = {
  accountAlias: string
  asset: string
  amount: string
  description?: string
  chartOfAccounts?: string
  metadata: MetadataDto
}

export type CreateTransactionDto = {
  description?: string
  chartOfAccountsGroupName?: string
  amount: string
  asset: string
  source: CreateTransactionOperationDto[]
  destination: CreateTransactionOperationDto[]
  metadata: MetadataDto
}

export type UpdateTransactionDto = {
  description?: string
  metadata?: Record<string, unknown>
}

export type TransactionOperationDto = CreateTransactionOperationDto

export type TransactionDto = {
  id: string
  ledgerId: string
  organizationId: string
  description?: string
  chartOfAccountsGroupName?: string
  status: StatusDto
  amount: string
  asset: string
  source: TransactionOperationDto[]
  destination: TransactionOperationDto[]
  metadata: MetadataDto
  createdAt: string
  updatedAt?: string
  deletedAt?: string
}
