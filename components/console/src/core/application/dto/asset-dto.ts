import { MetadataDto } from './metadata-dto'
import { StatusDto } from './status-dto'

export type CreateAssetDto = {
  name: string
  type: string
  code: string
  status?: StatusDto
  metadata?: MetadataDto
}

export type UpdateAssetDto = Omit<Partial<CreateAssetDto>, 'type' | 'code'>

export type AssetResponseDto = {
  id: string
  organizationId: string
  ledgerId: string
  name: string
  type: string
  code: string
  metadata: Record<string, string> | null
  createdAt: Date
  updatedAt: Date
  deletedAt: Date | null
}
