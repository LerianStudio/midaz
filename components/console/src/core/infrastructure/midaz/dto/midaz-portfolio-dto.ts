import { MidazMetadataDto } from './midaz-metadata-dto'
import { MidazStatusDto } from './midaz-status-dto'

export type MidazCreatePortfolioDto = {
  name: string
  entityId?: string
  status?: MidazStatusDto
  metadata?: MidazMetadataDto
}

export type MidazUpdatePortfolioDto = Partial<MidazCreatePortfolioDto>

export type MidazPortfolioDto = MidazCreatePortfolioDto & {
  id: string
  ledgerId: string
  organizationId: string
  createdAt: Date
  updatedAt: Date
  deletedAt?: Date
}
