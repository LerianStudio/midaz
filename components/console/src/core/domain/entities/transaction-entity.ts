import { MetadataEntity } from './metadata-entity'
import { StatusEntity } from './status-entity'

type TransactionSourceDto = {
  account: string
  accountAlias?: string
  asset: string
  value: number
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
  value: number
  asset: string
  source: TransactionSourceDto[]
  destination: TransactionSourceDto[]
  metadata: MetadataEntity
  createdAt?: string
  updatedAt?: string
  deletedAt?: string
}
