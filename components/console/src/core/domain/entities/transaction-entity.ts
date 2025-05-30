import { MetadataEntity } from './metadata-entity'
import { StatusEntity } from './status-entity'

type AmountEntity = {
  value: number
  scale: number
}

type TransactionSourceDto = {
  account: string
  accountAlias?: string
  asset: string
  amount: AmountEntity
  description?: string
  chartOfAccounts?: string
  metadata: MetadataEntity
}

export type TransactionEntity = {
  id?: string
  ledgerId?: string
  organizationId?: string
  description?: string
  chartOfAccountsGroupName?: string
  status?: StatusEntity
  amount: AmountEntity
  asset: string
  source: TransactionSourceDto[]
  destination: TransactionSourceDto[]
  metadata: MetadataEntity
  createdAt?: string
  updatedAt?: string
  deletedAt?: string
}
