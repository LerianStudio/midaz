import { AssetEntity } from '@/core/domain/entities/asset-entity'
import { StatusDto } from './status-dto'
import { MetadataDto } from './metadata-dto'

export type CreateLedgerDto = {
  name: string
  status: StatusDto
  metadata?: MetadataDto
}

export type UpdateLedgerDto = Partial<CreateLedgerDto>

export type LedgerResponseDto = {
  id: string
  organizationId: string
  name: string
  status: StatusDto
  metadata: MetadataDto
  createdAt: Date
  updatedAt: Date
  deletedAt: Date | null
  assets?: AssetEntity[]
}
