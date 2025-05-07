import { MidazMetadataDto } from './midaz-metadata-dto'
import { MidazStatusDto } from './midaz-status-dto'

export type MidazCreateSegmentDto = {
  name: string
  status?: MidazStatusDto
  metadata?: MidazMetadataDto
}

export type MidazUpdateSegmentDto = Partial<MidazCreateSegmentDto>

export type MidazSegmentDto = MidazCreateSegmentDto & {
  id: string
  ledgerId: string
  organizationId: string
  createdAt: Date
  updatedAt: Date
  deletedAt?: Date
}
