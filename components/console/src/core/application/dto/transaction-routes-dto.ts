import { MetadataDto } from './metadata-dto'
import { CursorSearchParamDto } from './request-dto'
import { OperationRoutesDto } from './operation-routes-dto'

export type CreateTransactionRoutesDto = {
  title: string
  description?: string
  operationRoutes: string[]
  metadata?: MetadataDto
}

export type UpdateTransactionRoutesDto = Partial<CreateTransactionRoutesDto>

export type TransactionRoutesDto = {
  id: string
  organizationId: string
  ledgerId: string
  title: string
  description?: string
  operationRoutes: OperationRoutesDto[]
  metadata?: MetadataDto
  createdAt: string
  updatedAt: string
  deletedAt?: string
}

// New cursor-based search parameters (preferred)
export type TransactionRoutesSearchParamDto = CursorSearchParamDto & {
  id?: string
  sortBy?: 'id' | 'title' | 'createdAt' | 'updatedAt'
}
