import { AssetResponseDto } from './asset-response-dto'
import { StatusDto } from './status.dto'

export interface LedgersViewResponseDTO {
  id: string
  organizationId: string
  name: string
  status: StatusDto
  metadata: Record<string, any>
  createdAt: Date
  updatedAt: Date
  deletedAt: Date | null
  assets: AssetResponseDto[]
}
