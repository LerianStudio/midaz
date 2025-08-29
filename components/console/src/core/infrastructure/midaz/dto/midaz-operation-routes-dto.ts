import { MetadataEntity } from '@/core/domain/entities/metadata-entity'
import { MidazMetadataDto } from './midaz-metadata-dto'

export type MidazCreateOperationRoutesDto = {
  title: string
  description: string
  operationType?: 'source' | 'destination'
  account?: {
    ruleType: string
    validIf: string | string[] | number | boolean | object | null | any
  }
  metadata?: MidazMetadataDto
}

export type MidazUpdateOperationRoutesDto =
  Partial<MidazCreateOperationRoutesDto>

export type MidazOperationRoutesDto = {
  id: string
  organizationId: string
  ledgerId: string
  title: string
  description: string
  operationType?: 'source' | 'destination'
  account?: {
    ruleType: string
    validIf: string | string[] | number | boolean | object | null | any
  }
  metadata?: MetadataEntity
  createdAt: Date
  updatedAt: Date
  deletedAt?: Date
}
