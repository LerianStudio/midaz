import { MidazFetchAllSegmentsRepository } from './midaz-fetch-all-segments-repository'
import { SegmentEntity } from '@/core/domain/entities/segment-entity'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { HTTP_METHODS } from '../../utils/http-fetch-utils'

jest.mock('../../utils/http-fetch-utils', () => ({
  httpMidazFetch: jest.fn(),
  HTTP_METHODS: {
    GET: 'GET'
  }
}))

describe('MidazFetchAllSegmentsRepository', () => {
  let repository: MidazFetchAllSegmentsRepository
  let mockHttpFetchUtils: { httpMidazFetch: jest.Mock }

  beforeEach(() => {
    mockHttpFetchUtils = { httpMidazFetch: jest.fn() }
    repository = new MidazFetchAllSegmentsRepository(mockHttpFetchUtils as any)
    jest.clearAllMocks()
  })

  it('should fetch all segments successfully', async () => {
    const organizationId = '1'
    const ledgerId = '1'
    const limit = 10
    const page = 1
    const response: PaginationEntity<SegmentEntity> = {
      items: [
        {
          id: '1',
          name: 'Test Segment',
          status: { code: 'ACTIVE', description: '' },
          metadata: {}
        },
        {
          id: '2',
          name: 'Test Segment 2',
          status: { code: 'ACTIVE', description: '' },
          metadata: {}
        }
      ],
      limit,
      page
    }

    mockHttpFetchUtils.httpMidazFetch.mockResolvedValueOnce(response)

    const result = await repository.fetchAll(
      organizationId,
      ledgerId,
      limit,
      page
    )

    expect(mockHttpFetchUtils.httpMidazFetch).toHaveBeenCalledWith({
      url: `${process.env.MIDAZ_BASE_PATH}/organizations/${organizationId}/ledgers/${ledgerId}/segments?limit=${limit}&page=${page}`,
      method: HTTP_METHODS.GET
    })
    expect(result).toEqual(response)
  })

  it('should handle errors when fetching all segments', async () => {
    const organizationId = '1'
    const ledgerId = '1'
    const limit = 10
    const page = 1
    const error = new Error('Error occurred')

    mockHttpFetchUtils.httpMidazFetch.mockRejectedValueOnce(error)

    await expect(
      repository.fetchAll(organizationId, ledgerId, limit, page)
    ).rejects.toThrow('Error occurred')

    expect(mockHttpFetchUtils.httpMidazFetch).toHaveBeenCalledWith({
      url: `${process.env.MIDAZ_BASE_PATH}/organizations/${organizationId}/ledgers/${ledgerId}/segments?limit=${limit}&page=${page}`,
      method: HTTP_METHODS.GET
    })
  })
})
