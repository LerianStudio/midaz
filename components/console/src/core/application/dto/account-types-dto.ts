import { MetadataDto } from './metadata-dto'
import { CursorSearchParamDto } from './request-dto'

export type AccountTypesSearchParamDto = CursorSearchParamDto & {
  id?: string
  name?: string
  keyValue?: string
  sortBy?: 'id' | 'name' | 'createdAt' | 'updatedAt'
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
