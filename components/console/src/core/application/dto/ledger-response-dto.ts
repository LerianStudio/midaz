import { AssetEntity } from '@/core/domain/entities/asset-entity'
import { StatusDto } from './status.dto'

export interface LedgerResponseDto {
  id: string
  organizationId: string
  name: string
  status: StatusDto
  metadata: Record<string, any>
  createdAt: Date
  updatedAt: Date
  deletedAt: Date | null
  assets?: AssetEntity[]
}
