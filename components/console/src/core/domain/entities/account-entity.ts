import { MetadataEntity } from './metadata-entity'

export type AccountEntity = {
  id?: string
  ledgerId?: string
  organizationId?: string
  parentAccountId?: string
  segmentId?: string
  portfolioId?: string
  entityId?: string
  name: string
  alias?: string
  type: string
  assetCode: string
  metadata: MetadataEntity
  createdAt?: Date
  updatedAt?: Date
  deletedAt?: Date
}
