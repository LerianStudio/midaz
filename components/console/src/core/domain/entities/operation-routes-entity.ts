import { MetadataEntity } from "./metadata-entity"
import { PaginationSearchEntity } from "./pagination-entity"

export type OperationRoutesSearchEntity = PaginationSearchEntity & {
  id?: string
}

export type OperationRoutesEntity = {
  id?: string
  organizationId?: string
  ledgerId?: string
  title: string
  description: string
  operationType?: 'source' | 'destination'
  account?: {
    ruleType: string
    validIf: string | string[] | number | boolean | object | null | any
  }
  metadata?: MetadataEntity
  createdAt?: Date
  updatedAt?: Date
  deletedAt?: Date
}

