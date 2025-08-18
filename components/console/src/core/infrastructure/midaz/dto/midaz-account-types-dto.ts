import { MidazMetadataDto } from './midaz-metadata-dto'

export type MidazCreateAccountTypesDto = {
  name: string
  description?: string
  keyValue: string
  metadata?: MidazMetadataDto
}

export type MidazUpdateAccountTypesDto = Partial<Omit<MidazCreateAccountTypesDto, 'keyValue'>>

export type MidazAccountTypesDto = MidazCreateAccountTypesDto & {
  id: string
  ledgerId: string
  organizationId: string
  createdAt: Date
  updatedAt: Date
  deletedAt?: Date
}
