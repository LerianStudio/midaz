import { SegmentEntity } from '../../entities/segment-entity'

export abstract class FetchSegmentByIdRepository {
  abstract fetchById: (
    organizationId: string,
    ledgerId: string,
    segmentId: string
  ) => Promise<SegmentEntity>
}
