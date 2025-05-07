import { AssetEntity } from './asset-entity'
import { MetadataEntity } from './metadata-entity'

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
