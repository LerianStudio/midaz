import { MetadataDto } from './metadata-dto'

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

export interface AccountResponseDto {
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
  metadata: MetadataDto
  createdAt: Date
  updatedAt: Date
  deletedAt: Date | null
}
