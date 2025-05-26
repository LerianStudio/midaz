import { MidazMetadataDto } from './midaz-metadata-dto'
import { MidazStatusDto } from './midaz-status-dto'

export type MidazCreateAssetDto = {
  name: string
  type: string
  code: string
  status?: MidazStatusDto
  metadata?: MidazMetadataDto
}

export type MidazUpdateAssetDto = Omit<
  Partial<MidazCreateAssetDto>,
  'type' | 'code'
>

export type MidazAssetDto = MidazCreateAssetDto & {
  id: string
  organizationId: string
  ledgerId: string
  createdAt: Date
  updatedAt: Date
  deletedAt?: Date
}
