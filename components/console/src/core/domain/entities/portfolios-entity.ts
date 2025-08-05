import { MetadataEntity } from './metadata-entity'
import { PaginationSearchEntity } from './pagination-entity'

export type PortfolioSearchEntity = PaginationSearchEntity & {
  id?: string
}

export type PortfolioEntity = {
  id?: string
  ledgerId?: string
  organizationId?: string
  name: string
  entityId?: string
  metadata: MetadataEntity
  createdAt?: Date
  updatedAt?: Date
  deletedAt?: Date
}
