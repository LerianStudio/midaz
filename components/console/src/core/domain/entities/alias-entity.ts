import { MetadataEntity } from './metadata-entity'

export interface BankingDetailsEntity {
  branch: string
  account: string
  type: string
  countryCode: string
  bankId: string
}

export interface AliasEntity {
  id: string
  document: string
  type: string
  ledgerId: string
  accountId: string
  holderId: string
  metadata?: MetadataEntity
  bankingDetails?: BankingDetailsEntity
  createdAt: string
  updatedAt: string
  deletedAt?: string | null
}

export interface CreateAliasEntity {
  document: string
  type: string
  ledgerId: string
  accountId: string
  holderId: string
  metadata?: MetadataEntity
  bankingDetails?: BankingDetailsEntity
}

export interface UpdateAliasEntity {
  document?: string
  type?: string
  metadata?: MetadataEntity
  bankingDetails?: BankingDetailsEntity
}
