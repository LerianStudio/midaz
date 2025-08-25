import { MetadataDto } from './metadata-dto'
import { SearchParamDto } from './request-dto'

export type AccountTypesSearchParamDto = SearchParamDto & {
  id?: string
  name?: string
  keyValue?: string
}

export type CreateAccountTypesDto = {
  name: string
  description?: string
  keyValue: string
  metadata?: MetadataDto
}

export type UpdateAccountTypesDto = Partial<
  Omit<CreateAccountTypesDto, 'keyValue'>
>

export interface AccountTypesDto {
  id: string
  ledgerId: string
  organizationId: string
  name: string
  description?: string
  keyValue: string
  metadata: MetadataDto
  createdAt: Date
  updatedAt: Date
  deletedAt: Date | null
}
