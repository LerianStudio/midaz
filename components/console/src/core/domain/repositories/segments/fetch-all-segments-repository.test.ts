import { FetchAllSegmentsRepository } from './fetch-all-segments-repository'
import { PaginationEntity } from '../../entities/pagination-entity'
import { SegmentEntity } from '../../entities/segment-entity'

describe('FetchAllSegmentsRepository', () => {
  let fetchAllSegmentsRepository: FetchAllSegmentsRepository

  beforeEach(() => {
    fetchAllSegmentsRepository = {
      fetchAll: jest.fn()
    }
  })

  it('should fetch all segments with given organizationId and ledgerId', async () => {
    const organizationId = 'org123'
    const ledgerId = 'ledger123'
    const limit = 10
    const page = 1
    const expectedSegments: PaginationEntity<SegmentEntity> = {
      items: [
        {
          id: '1',
          name: 'Test Segment',
          organizationId: 'org123',
          ledgerId: 'ledger123',
          metadata: { key: 'value' },
          status: { code: 'active', description: 'Active' },
          createdAt: new Date(),
          updatedAt: new Date(),
          deletedAt: null
        },
        {
          id: '2',
          name: 'Test Segment 2',
          organizationId: 'org123',
          ledgerId: 'ledger123',
          metadata: { key: 'value' },
          status: { code: 'active', description: 'Active' },
          createdAt: new Date(),
          updatedAt: new Date(),
          deletedAt: null
        }
      ],
      page: 1,
      limit: 10
    }
    ;(fetchAllSegmentsRepository.fetchAll as jest.Mock).mockResolvedValue(
      expectedSegments
    )

    const result = await fetchAllSegmentsRepository.fetchAll(
      organizationId,
      ledgerId,
      limit,
      page
    )

    expect(fetchAllSegmentsRepository.fetchAll).toHaveBeenCalledWith(
      organizationId,
      ledgerId,
      limit,
      page
    )
    expect(result).toEqual(expectedSegments)
  })

  it('should handle errors when fetching segments', async () => {
    const organizationId = 'org123'
    const ledgerId = 'ledger123'
    const limit = 10
    const page = 1
    const error = new Error('Failed to fetch segments')

    ;(fetchAllSegmentsRepository.fetchAll as jest.Mock).mockRejectedValue(error)

    await expect(
      fetchAllSegmentsRepository.fetchAll(organizationId, ledgerId, limit, page)
    ).rejects.toThrow('Failed to fetch segments')
  })
})
