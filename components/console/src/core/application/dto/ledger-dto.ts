import { MetadataDto } from './metadata-dto'
import { AssetDto } from './asset-dto'
import { SearchParamDto } from './request-dto'

export type LedgerSearchParamDto = SearchParamDto & {
  id?: string
}

export type CreateLedgerDto = {
  name: string
  metadata?: MetadataDto
}

export type UpdateLedgerDto = Partial<CreateLedgerDto>

export type LedgerDto = {
  id: string
  organizationId: string
  name: string
  metadata: MetadataDto
  createdAt: Date
  updatedAt: Date
  deletedAt: Date | null
  assets?: AssetDto[]
}
