import { AccountResponseDto } from './account-dto'
import { MetadataDto } from './metadata-dto'
import { StatusDto } from './status-dto'

export type CreatePortfolioDto = {
  entityId: string
  ledgerId: string
  organizationId: string
  name: string
  status?: StatusDto
  metadata?: MetadataDto
}

export type UpdatePortfolioDto = {
  name?: string
  status?: StatusDto
  metadata?: MetadataDto
}

export type PortfolioResponseDto = {
  id: string
  ledgerId: string
  organizationId: string
  entityId: string
  name: string
  status: StatusDto
  metadata: MetadataDto
  createdAt: Date
  updatedAt: Date
  deletedAt: Date | null
  accounts?: AccountResponseDto[]
}
