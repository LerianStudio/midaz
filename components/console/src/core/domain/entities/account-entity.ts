import { StatusEntity } from './status-entity'

export type AccountEntity = {
  id?: string
  ledgerId?: string
  organizationId?: string
  parentAccountId?: string | null
  segmentId?: string | null
  portfolioId?: string | null
  entityId?: string | null
  name: string
  alias: string
  type: string
  assetCode: string
  status: StatusEntity
  metadata: Record<string, any>
  createdAt?: Date
  updatedAt?: Date
  deletedAt?: Date | null
}
