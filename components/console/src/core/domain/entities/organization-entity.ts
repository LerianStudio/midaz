import { MetadataEntity } from './metadata-entity'
import { PaginationSearchEntity } from './pagination-entity'

export type OrganizationSearchEntity = PaginationSearchEntity & {
  id?: string
}

type Address = {
  line1: string
  line2?: string
  neighborhood: string
  zipCode: string
  city: string
  state: string
  country: string
}

export type OrganizationEntity = {
  id?: string
  parentOrganizationId?: string
  legalName: string
  doingBusinessAs?: string
  legalDocument: string
  address: Address
  metadata?: MetadataEntity
  createdAt?: Date
  updatedAt?: Date
  deletedAt?: Date
}
