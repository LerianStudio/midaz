import { PaginationEntity } from '../../entities/pagination-entity'
import { SegmentEntity } from '../../entities/segment-entity'

export abstract class FetchAllSegmentsRepository {
  abstract fetchAll: (
    organizationId: string,
    ledgerId: string,
    limit: number,
    page: number
  ) => Promise<PaginationEntity<SegmentEntity>>
}
