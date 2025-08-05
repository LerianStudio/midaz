import { AssetEntity } from './asset-entity'
import { MetadataEntity } from './metadata-entity'
import { PaginationSearchEntity } from './pagination-entity'

export type LedgerSearchEntity = PaginationSearchEntity & {
  id?: string
}

export type LedgerEntity = {
  id?: string
  organizationId?: string
  name: string
  metadata: MetadataEntity
  createdAt?: Date
  updatedAt?: Date
  deletedAt?: Date | null
  assets?: AssetEntity[]
}
