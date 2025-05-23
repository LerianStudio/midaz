import { AccountResponseDto } from '@/core/application/dto/account-dto'
import { StatusDto } from '@/core/application/dto/status.dto'

export type PortfolioType = {
  id: string
  ledgerId: string
  organizationId: string
  entityId: string
  name: string
  status: StatusDto
  metadata: Record<string, any>
  createdAt: Date
  updatedAt: Date
  deletedAt: Date | null
  accounts?: AccountResponseDto[]
}
