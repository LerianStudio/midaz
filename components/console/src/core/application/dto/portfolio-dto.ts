import { type AccountDto } from './account-dto'
import { MetadataDto } from './metadata-dto'
import { SearchParamDto } from './request-dto'

export type PortfolioSearchParamDto = SearchParamDto & {
  id?: string
}

export type CreatePortfolioDto = {
  entityId: string
  ledgerId: string
  organizationId: string
  name: string
  metadata?: MetadataDto
}

export type UpdatePortfolioDto = {
  name?: string
  metadata?: MetadataDto
}

export type PortfolioDto = {
  id: string
  ledgerId: string
  organizationId: string
  entityId: string
  name: string
  metadata: MetadataDto
  createdAt: Date
  updatedAt: Date
  deletedAt: Date | null
  accounts?: AccountDto[]
}
