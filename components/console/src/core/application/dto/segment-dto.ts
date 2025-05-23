import { StatusDto } from './status.dto'

export interface CreateSegmentDto {
  name: string
  status: StatusDto
  metadata: Record<string, any>
}

export interface SegmentResponseDto {
  id: string
  ledgerId: string
  organizationId: string
  name: string
  status: StatusDto
  metadata: Record<string, any>
  createdAt: Date
  updatedAt: Date
  deletedAt: Date | null
}

export interface UpdateSegmentDto {
  name?: string
  status?: StatusDto
  metadata?: Record<string, any>
}
