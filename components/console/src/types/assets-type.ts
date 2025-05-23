import { StatusDto } from '@/core/application/dto/status.dto'

type Metadata = Record<string, string>

export type AssetType = {
  id: string
  ledgerId: string
  organizationId: string
  name: string
  type: string
  code: string
  status: StatusDto
  metadata: Metadata | null
  createdAt: Date
  updatedAt: Date
  deletedAt: Date | null
}
