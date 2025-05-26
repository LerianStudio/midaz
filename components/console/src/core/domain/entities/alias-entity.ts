import { MetadataEntity } from './metadata-entity'

export interface BankAccountEntity {
  bankCode: string
  branch: string
  number: string
  type: string
  holderName: string
}

export interface AliasEntity {
  id: string
  name: string
  type: string
  ledgerId: string
  accountId: string
  metadata?: MetadataEntity
  bankAccount?: BankAccountEntity
  createdAt: string
  updatedAt: string
  deletedAt?: string
}

export interface CreateAliasEntity {
  name: string
  type: string
  ledgerId: string
  accountId: string
  metadata?: MetadataEntity
  bankAccount?: BankAccountEntity
}

export interface UpdateAliasEntity {
  name?: string
  type?: string
  metadata?: MetadataEntity
  bankAccount?: BankAccountEntity
}
