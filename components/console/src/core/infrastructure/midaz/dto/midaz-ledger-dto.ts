import { MidazStatusDto } from './midaz-status-dto'
import { MidazMetadataDto } from './midaz-metadata-dto'

export type MidazCreateLedgerDto = {
  name: string
  status?: MidazStatusDto
  metadata?: MidazMetadataDto
}

export type MidazUpdateLedgerDto = Partial<MidazCreateLedgerDto>

export type MidazLedgerDto = MidazCreateLedgerDto & {
  id: string
  organizationId: string
  createdAt: Date
  updatedAt: Date
  deletedAt?: Date
}
