import { AccountResponseDto } from './account-dto'
import { StatusDto } from './status.dto'

export interface PortfolioViewResponseDTO {
  id: string
  organizationId: string
  ledgerId: string
  name: string
  status: StatusDto
  metadata: Record<string, any>
  createdAt: Date
  updatedAt: Date
  deletedAt: Date | null
  accounts: AccountResponseDto[]
}
