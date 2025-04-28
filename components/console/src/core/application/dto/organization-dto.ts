import { AddressDto } from './address-dto'
import { MetadataDto } from './metadata-dto'
import { StatusDto } from './status-dto'

export type CreateOrganizationDto = {
  legalName: string
  parentOrganizationId?: string
  doingBusinessAs?: string
  legalDocument: string
  address: AddressDto
  metadata?: MetadataDto
  status: StatusDto
}

export type UpdateOrganizationDto = {
  legalName?: string
  parentOrganizationId?: string
  doingBusinessAs?: string
  address?: AddressDto
  status?: StatusDto
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
  status: StatusDto
  createdAt: Date
  updatedAt: Date
  deletedAt?: Date
}
