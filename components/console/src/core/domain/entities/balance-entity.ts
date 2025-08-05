export type BalanceEntity = {
  id?: string
  organizationId?: string
  ledgerId?: string
  accountId?: string
  alias?: string
  assetCode?: string
  available?: string
  onHold?: string
  allowSending?: boolean
  allowReceiving?: boolean
  createdAt?: Date
  updatedAt?: Date
  deletedAt?: Date | null
}
