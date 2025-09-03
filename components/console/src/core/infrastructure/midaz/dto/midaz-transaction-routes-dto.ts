import { MetadataEntity } from '@/core/domain/entities/metadata-entity'
import { MidazMetadataDto } from './midaz-metadata-dto'
import { MidazOperationRoutesDto } from './midaz-operation-routes-dto'

export type MidazCreateTransactionRoutesDto = {
  title: string
  description?: string
  operationRoutes: string[]
  metadata?: MidazMetadataDto
}

export type MidazUpdateTransactionRoutesDto =
  Partial<MidazCreateTransactionRoutesDto>

export type MidazTransactionRoutesDto = {
  id: string
  organizationId: string
  ledgerId: string
  title: string
  description?: string
  operationRoutes?: MidazOperationRoutesDto[]
  metadata?: MetadataEntity
  createdAt: Date
  updatedAt: Date
  deletedAt?: Date
}
