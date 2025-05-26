import { PortfolioType } from './portfolio-type'

export type AccountType = {
  id: string
  ledgerId: string
  assetCode: string
  organizationId: string
  name: string
  alias?: string
  type?: string
  entityId?: string
  parentAccountId: string
  portfolioId?: string | null
  portfolio: Pick<PortfolioType, 'name'>
  portfolioName?: string
  segmentId: string
  status: {
    code: string
    description: string
  }
  metadata?: Record<string, any>
  createdAt: Date
  updatedAt: Date
  deletedAt: Date | null
}
