import { AddressDto } from './address-dto'
import { StatusDto } from './status.dto'

export interface CreateOrganizationDto {
  legalName: string
  parentOrganizationId?: string
  doingBusinessAs?: string
  legalDocument: string
  address: AddressDto
  metadata?: Record<string, any>
  status: StatusDto
}
