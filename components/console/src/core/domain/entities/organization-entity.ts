import { StatusEntity } from './status-entity'

export type OrganizationEntity = {
  id?: string
  parentOrganizationId?: string
  legalName: string
  doingBusinessAs?: string
  legalDocument: string
  address: Address
  metadata?: Record<string, any>
  status: StatusEntity
  createdAt?: Date
  updatedAt?: Date
  deletedAt?: Date
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
