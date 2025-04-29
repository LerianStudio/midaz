import { AddressDto } from './midaz-address-metadata-dto'
import { MidazMetadataDto } from './midaz-metadata-dto'
import { MidazStatusDto } from './midaz-status-dto'

export type MidazCreateOrganizationDto = {
  legalName: string
  parentOrganizationId?: string
  doingBusinessAs?: string
  legalDocument: string
  address: AddressDto
  metadata?: MidazMetadataDto
  status?: MidazStatusDto
}

export type MidazUpdateOrganizationDto = Partial<
  Omit<MidazCreateOrganizationDto, 'doingBusinessAs'>
>

export type MidazOrganizationDto = MidazCreateOrganizationDto & {
  id: string
  createdAt: Date
  updatedAt: Date
  deletedAt?: Date
}
