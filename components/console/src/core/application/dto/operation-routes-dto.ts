import { MetadataDto } from './metadata-dto'
import { SearchParamDto } from './request-dto'

export type CreateOperationRoutesDto = {
  title: string
  description: string
  operationType: 'source' | 'destination' 
  account?: {
    ruleType: string
    validIf: string | string[] | number | boolean | object | null
  }
  metadata?: MetadataDto
}

export type UpdateOperationRoutesDto = Partial<CreateOperationRoutesDto>

export type OperationRoutesDto = {
  id: string
  organizationId: string
  ledgerId: string
  title: string
  description: string
  account?: {
    ruleType: string
    validIf: string | string[] | number | boolean | object | null
  }
  metadata?: MetadataDto
  createdAt: string
  updatedAt: string
  deletedAt?: string
}

export type OperationRoutesSearchParamDto = SearchParamDto & {
  id?: string
}