import { StatusDto } from '@/core/application/dto/status.dto'

export interface LedgerType {
  id: string
  organizationId: string
  name: string
  status: StatusDto
  metadata: Record<string, any> | null
  createdAt: Date
  updatedAt: Date
  deletedAt: Date | null
}
