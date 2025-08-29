// Types for SearchAccountByAliasField component

export interface AccountDto {
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
  allowSending?: boolean
  allowReceiving?: boolean
  metadata: any
  createdAt: Date
  updatedAt: Date
  deletedAt: Date | null
}

export type { AccountDto as Account }
