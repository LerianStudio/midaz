import { MetadataDto } from './metadata-dto'
import { SearchParamDto } from './request-dto'
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

export type TransactionRoutesSearchParamDto = SearchParamDto & {
  id?: string
}
