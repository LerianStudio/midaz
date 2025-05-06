import { AddressDto } from './address-dto'
import { MetadataDto } from './metadata-dto'

export type CreateOrganizationDto = {
  legalName: string
  parentOrganizationId?: string
  doingBusinessAs?: string
  legalDocument: string
  address: AddressDto
  metadata?: MetadataDto
}

export type UpdateOrganizationDto = {
  legalName?: string
  parentOrganizationId?: string
  doingBusinessAs?: string
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
  createdAt: Date
  updatedAt: Date
  deletedAt?: Date
}
