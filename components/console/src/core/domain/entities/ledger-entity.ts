import { AssetEntity } from './asset-entity'
import { StatusEntity } from './status-entity'

export type LedgerEntity = {
  id?: string
  organizationId?: string
  name: string
  status: StatusEntity
  metadata: Record<string, string> | null
  createdAt?: Date
  updatedAt?: Date
  deletedAt?: Date | null
  assets?: AssetEntity[]
}
