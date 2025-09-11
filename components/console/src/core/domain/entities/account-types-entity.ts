import { MetadataEntity } from './metadata-entity'
import { CursorSortableSearchEntity } from './pagination-entity'

export type AccountTypesSearchEntity = CursorSortableSearchEntity & {
  id?: string
  name?: string
  keyValue?: string
  sortBy?: 'id' | 'name' | 'createdAt' | 'updatedAt'
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
