import { MetadataDto } from './metadata-dto'

export type CreateSegmentDto = {
  name: string
  metadata?: MetadataDto
}

export type UpdateSegmentDto = Partial<CreateSegmentDto>

export type SegmentDto = {
  id: string
  ledgerId: string
  organizationId: string
  name: string
  metadata: MetadataDto
  createdAt: Date
  updatedAt: Date
  deletedAt: Date | null
}
