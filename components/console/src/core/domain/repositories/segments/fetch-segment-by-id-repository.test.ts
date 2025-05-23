import { FetchSegmentByIdRepository } from './fetch-segment-by-id-repository'
import { SegmentEntity } from '../../entities/segment-entity'

const segment: SegmentEntity = {
  id: '1',
  name: 'Test Segment',
  organizationId: 'org123',
  ledgerId: 'ledger123',
  metadata: { key: 'value' },
  status: { code: 'active', description: 'Active' },
  createdAt: new Date(),
  updatedAt: new Date(),
  deletedAt: null
}

class MockFetchSegmentByIdRepository implements FetchSegmentByIdRepository {
  fetchById(
    organizationId: string,
    ledgerId: string,
    segmentId: string
  ): Promise<SegmentEntity> {
    return Promise.resolve(segment)
  }
}

describe('FetchSegmentByIdRepository', () => {
  let repository: FetchSegmentByIdRepository

  beforeEach(() => {
    repository = new MockFetchSegmentByIdRepository()
  })

  it('should fetch segment by id', async () => {
    const organizationId = 'org123'
    const ledgerId = 'ledger123'
    const segmentId = '1'

    const segment = await repository.fetchById(
      organizationId,
      ledgerId,
      segmentId
    )

    expect(segment).toEqual(segment)
  })

  it('should return a segment with the correct id', async () => {
    const organizationId = 'org123'
    const ledgerId = 'ledger123'
    const segmentId = '1'

    const segment = await repository.fetchById(
      organizationId,
      ledgerId,
      segmentId
    )

    expect(segment.id).toBe(segmentId)
  })
})
