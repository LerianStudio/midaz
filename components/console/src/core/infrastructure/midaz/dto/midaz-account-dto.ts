import { MidazMetadataDto } from './midaz-metadata-dto'
import { MidazStatusDto } from './midaz-status-dto'

export type MidazCreateAccountDto = {
  name: string
  alias?: string
  assetCode: string
  type: string
  entityId?: string | null
  parentAccountId?: string | null
  portfolioId?: string | null
  segmentId?: string
  status?: MidazStatusDto
  metadata?: MidazMetadataDto
}

export type MidazUpdateAccountDto = Partial<MidazCreateAccountDto>

export type MidazAccountDto = MidazCreateAccountDto & {
  id: string
  ledgerId: string
  organizationId: string
  createdAt: Date
  updatedAt: Date
  deletedAt?: Date
}
