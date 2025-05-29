export type BalanceDto = {
  id: string
  organizationId: string
  ledgerId: string
  accountId: string
  alias: string
  assetCode: string
  available: number
  onHold: number
  scale: number
  allowSending: boolean
  allowReceiving: boolean
  createdAt: Date
  updatedAt: Date
  deletedAt?: Date | null
}
