import { AccountResponseDto } from './account-dto'
import { StatusDto } from './status.dto'

export interface CreatePortfolioDto {
  entityId: string
  ledgerId: string
  organizationId: string
  name: string
  status: StatusDto
  metadata: Record<string, any>
}

export interface PortfolioResponseDto {
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

export interface UpdatePortfolioDto {
  name?: string
  status?: StatusDto
  metadata?: Record<string, any>
}
