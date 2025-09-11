import { MetadataEntity } from './metadata-entity'
import { CursorSortableSearchEntity } from './pagination-entity'
import { OperationRoutesEntity } from './operation-routes-entity'

// New cursor-based search entity (preferred)
export type TransactionRoutesSearchEntity = CursorSortableSearchEntity & {
  id?: string
  sortBy?: 'id' | 'title' | 'createdAt' | 'updatedAt'
}

export type TransactionRoutesEntity = {
  id?: string
  organizationId?: string
  ledgerId?: string
  title: string
  description?: string
  operationRoutes: OperationRoutesEntity[]
  metadata?: MetadataEntity
  createdAt?: Date
  updatedAt?: Date
  deletedAt?: Date
}
