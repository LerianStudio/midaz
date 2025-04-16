import { AddressDto } from './address-dto'
import { StatusDto } from './status.dto'

export interface UpdateOrganizationDto {
  legalName?: string
  parentOrganizationId?: string
  doingBusinessAs?: string
  address?: AddressDto
  status?: StatusDto
  metadata?: Record<string, any>
}
