import { MetadataEntity } from './metadata-entity'

export type { MetadataEntity }

export interface AddressEntity {
  line1: string
  line2?: string
  zipCode: string
  city: string
  state: string
  country: string
}

export interface ContactEntity {
  name: string
  value: string
}

export interface HolderEntity {
  id: string
  name: string
  status: string
  type: 'NATURAL_PERSON' | 'LEGAL_PERSON'
  document: string
  address?: AddressEntity
  tradingName?: string
  legalName?: string
  website?: string
  establishedOn?: string
  monthlyIncomeTotal?: number
  contacts?: ContactEntity[]
  metadata?: MetadataEntity
  createdAt: string
  updatedAt: string
  deletedAt?: string
}

export interface CreateHolderEntity {
  name: string
  type: 'NATURAL_PERSON' | 'LEGAL_PERSON'
  document: string
  status?: string
  address?: AddressEntity
  tradingName?: string
  legalName?: string
  website?: string
  establishedOn?: string
  monthlyIncomeTotal?: number
  contacts?: ContactEntity[]
  metadata?: MetadataEntity
}

export interface UpdateHolderEntity {
  name?: string
  status?: string
  address?: AddressEntity
  tradingName?: string
  legalName?: string
  website?: string
  establishedOn?: string
  monthlyIncomeTotal?: number
  contacts?: ContactEntity[]
  metadata?: MetadataEntity
}
