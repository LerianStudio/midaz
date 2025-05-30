import { MetadataDto } from './metadata-dto'
import { type PortfolioDto } from './portfolio-dto'
import { SearchParamDto } from './request-dto'

export type AccountSearchParamDto = SearchParamDto & {
  id?: string
  alias?: string
}

export type CreateAccountDto = {
  assetCode: string
  name: string
  alias: string
  type: string
  entityId?: string | null
  parentAccountId?: string | null
  portfolioId?: string | null
  segmentId?: string
  allowSending?: boolean
  allowReceiving?: boolean
  metadata?: MetadataDto
}

export type UpdateAccountDto = Partial<CreateAccountDto>

export interface AccountDto {
  id: string
  ledgerId: string
  assetCode: string
  organizationId: string
  name: string
  alias: string
  type: string
  entityId: string
  parentAccountId: string
  portfolioId?: string | null
  segmentId: string
  allowSending?: boolean
  allowReceiving?: boolean
  portfolio?: PortfolioDto
  metadata: MetadataDto
  createdAt: Date
  updatedAt: Date
  deletedAt: Date | null
}
