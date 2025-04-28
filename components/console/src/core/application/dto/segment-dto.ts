import { MetadataDto } from './metadata-dto'
import { StatusDto } from './status-dto'

export type CreateSegmentDto = {
  name: string
  status: StatusDto
  metadata?: MetadataDto
}

export type UpdateSegmentDto = Partial<CreateSegmentDto>

export type SegmentResponseDto = {
  id: string
  ledgerId: string
  organizationId: string
  name: string
  status: StatusDto
  metadata: MetadataDto
  createdAt: Date
  updatedAt: Date
  deletedAt: Date | null
}
