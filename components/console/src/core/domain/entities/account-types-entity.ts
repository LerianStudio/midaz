import { MetadataEntity } from './metadata-entity'
import { PaginationSearchEntity } from './pagination-entity'

export type AccountTypesSearchEntity = PaginationSearchEntity & {
  id?: string
  name?: string
  keyValue?: string
}

export type AccountTypesEntity = {
  id?: string
  ledgerId?: string
  organizationId?: string
  name: string
  description?: string
  keyValue: string
  metadata: MetadataEntity
  createdAt?: Date
  updatedAt?: Date
  deletedAt?: Date
}
