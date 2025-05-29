import { PaginationEntity } from '../entities/pagination-entity'
import { SegmentEntity } from '../entities/segment-entity'

export abstract class SegmentRepository {
  abstract create: (
    organizationId: string,
    ledgerId: string,
    segment: SegmentEntity
  ) => Promise<SegmentEntity>
  abstract fetchAll: (
    organizationId: string,
    ledgerId: string,
    limit: number,
    page: number
  ) => Promise<PaginationEntity<SegmentEntity>>
  abstract fetchById: (
    organizationId: string,
    ledgerId: string,
    segmentId: string
  ) => Promise<SegmentEntity>
  abstract update: (
    organizationId: string,
    ledgerId: string,
    segmentId: string,
    segment: Partial<SegmentEntity>
  ) => Promise<SegmentEntity>
  abstract delete: (
    organizationId: string,
    ledgerId: string,
    segmentId: string
  ) => Promise<void>
}
