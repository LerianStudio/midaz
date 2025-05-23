import { SegmentEntity } from '../../entities/segment-entity'

export abstract class CreateSegmentRepository {
  abstract create: (
    organizationId: string,
    ledgerId: string,
    segment: SegmentEntity
  ) => Promise<SegmentEntity>
}
