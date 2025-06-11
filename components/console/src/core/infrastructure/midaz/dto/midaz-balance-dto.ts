export type MidazUpdateBalanceDto = {
  allowSending?: boolean
  allowReceiving?: boolean
}

export type MidazBalanceDto = {
  id: string
  organizationId: string
  ledgerId: string
  accountId: string
  alias: string
  assetCode: string
  available: string
  onHold: string
  version: number
  accountType: string
  allowSending: boolean
  allowReceiving: boolean
  createdAt: Date
  updatedAt: Date
  deletedAt?: Date
}
