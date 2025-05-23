import { SegmentEntity } from '../../entities/segment-entity'

export abstract class UpdateSegmentRepository {
  abstract update: (
    organizationId: string,
    ledgerId: string,
    segmentId: string,
    segment: Partial<SegmentEntity>
  ) => Promise<SegmentEntity>
}
