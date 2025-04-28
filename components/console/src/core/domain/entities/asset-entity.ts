import { StatusDto } from '@/core/application/dto/status-dto'
import { StatusEntity } from './status-entity'

export interface AssetEntity {
  id?: string
  organizationId?: string
  ledgerId?: string
  name: string
  type: string
  code: string
  status: StatusEntity
  metadata: Record<string, string> | null
  createdAt?: Date
  updatedAt?: Date
  deletedAt?: Date | null
}
