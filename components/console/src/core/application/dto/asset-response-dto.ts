import { StatusDto } from './status.dto'

export interface AssetResponseDto {
  id: string
  organizationId: string
  ledgerId: string
  name: string
  type: string
  code: string
  status: StatusDto
  metadata: Record<string, string> | null
  createdAt: Date
  updatedAt: Date
  deletedAt: Date | null
}
