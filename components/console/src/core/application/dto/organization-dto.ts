import { AddressDto } from './address-dto'
import { MetadataDto } from './metadata-dto'

export type CreateOrganizationDto = {
  legalName: string
  parentOrganizationId?: string
  doingBusinessAs?: string
  legalDocument: string
  address: AddressDto
  metadata?: MetadataDto
  avatar?: string
}

export type UpdateOrganizationDto = {
  legalName?: string
  parentOrganizationId?: string
  doingBusinessAs?: string
  avatar?: string
  address?: AddressDto
  metadata?: MetadataDto
}

export type OrganizationResponseDto = {
  id: string
  legalName: string
  parentOrganizationId?: string
  doingBusinessAs?: string
  legalDocument: string
  address: AddressDto
  metadata?: MetadataDto
  avatar?: string
  createdAt: Date
  updatedAt: Date
  deletedAt?: Date
}
