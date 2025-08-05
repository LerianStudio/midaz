import { MetadataEntity } from './metadata-entity'
import { PaginationSearchEntity } from './pagination-entity'

export type SegmentSearchEntity = PaginationSearchEntity & {
  id?: string
}

export type SegmentEntity = {
  id?: string
  ledgerId?: string
  organizationId?: string
  name: string
  metadata: MetadataEntity
  createdAt?: Date
  updatedAt?: Date
  deletedAt?: Date | null
}
