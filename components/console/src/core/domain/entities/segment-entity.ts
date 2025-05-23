import { MetadataEntity } from './metadata-entity'

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
