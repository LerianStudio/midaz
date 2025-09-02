import { MetadataEntity } from './metadata-entity'
import { PaginationSearchEntity } from './pagination-entity'
import { OperationRoutesEntity } from './operation-routes-entity'

export type TransactionRoutesSearchEntity = PaginationSearchEntity & {
  id?: string
}

export type TransactionRoutesEntity = {
  id?: string
  organizationId?: string
  ledgerId?: string
  title: string
  description?: string
  operationRoutes: string[] | OperationRoutesEntity[]
  metadata?: MetadataEntity
  createdAt?: Date
  updatedAt?: Date
  deletedAt?: Date
}
