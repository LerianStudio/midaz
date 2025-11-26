import { MetadataEntity } from './metadata-entity'
import { CursorSortableSearchEntity } from './pagination-entity'

export type OperationRoutesSearchEntity = CursorSortableSearchEntity & {
  id?: string
  title?: string
  sortBy?: 'id' | 'title' | 'createdAt' | 'updatedAt'
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
  code?: string
  metadata?: MetadataEntity
  createdAt?: Date
  updatedAt?: Date
  deletedAt?: Date
}
