import { MetadataEntity } from './metadata-entity'

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
