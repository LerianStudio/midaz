import { MetadataDto } from './metadata-dto'
import { CursorSearchParamDto } from './request-dto'

export type CreateOperationRoutesDto = {
  title: string
  description: string
  operationType: 'source' | 'destination'
  account?: {
    ruleType: string
    validIf: string | string[] | number | boolean | object | null | any
  }
  code?: string
  metadata?: MetadataDto
}

export type UpdateOperationRoutesDto = Partial<CreateOperationRoutesDto>

export type OperationRoutesDto = {
  id: string
  organizationId: string
  ledgerId: string
  title: string
  description: string
  operationType: 'source' | 'destination'
  account?: {
    ruleType: string
    validIf: string | string[] | number | boolean | object | null | any
  }
  code?: string
  metadata?: MetadataDto
  createdAt: string
  updatedAt: string
  deletedAt?: string
}

export type OperationRoutesSearchParamDto = CursorSearchParamDto & {
  id?: string
  title?: string
  sortBy?: 'id' | 'title' | 'createdAt' | 'updatedAt'
}
