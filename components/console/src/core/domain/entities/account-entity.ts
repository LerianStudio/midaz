import { MetadataEntity } from './metadata-entity'

export type AccountEntity = {
  id?: string
  ledgerId?: string
  organizationId?: string
  parentAccountId?: string | null
  segmentId?: string | null
  portfolioId?: string | null
  entityId?: string | null
  name: string
  alias: string
  type: string
  assetCode: string
  metadata: MetadataEntity
  createdAt?: Date
  updatedAt?: Date
  deletedAt?: Date | null
}
