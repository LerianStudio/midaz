import { StatusDto } from './status.dto'

export interface CreateAccountDto {
  assetCode: string
  name: string
  alias: string
  type: string
  entityId?: string | null
  parentAccountId?: string | null
  portfolioId?: string | null
  segmentId?: string
  allowSending?: boolean
  allowReceiving?: boolean
  status: {
    code: string
    description: string
  }
  metadata?: Record<string, any>
}

export interface AccountResponseDto {
  id: string
  ledgerId: string
  assetCode: string
  organizationId: string
  name: string
  alias: string
  type: string
  entityId: string
  parentAccountId: string
  portfolioId?: string | null
  segmentId: string
  status: StatusDto
  allowSending?: boolean
  allowReceiving?: boolean
  metadata: Record<string, any>
  createdAt: Date
  updatedAt: Date
  deletedAt: Date | null
}

export interface UpdateAccountDto {
  assetCode?: string
  name?: string
  alias?: string
  type?: string
  entityId?: string
  parentAccountId?: string
  segmentId?: string
  portfolioId?: string
  allowSending?: boolean
  allowReceiving?: boolean
  status?: StatusDto
  metadata?: Record<string, any>
}
