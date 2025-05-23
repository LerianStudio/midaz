import { MidazFetchSegmentByIdRepository } from './midaz-fetch-segment-by-id-repository'
import { SegmentEntity } from '@/core/domain/entities/segment-entity'
import { HTTP_METHODS } from '../../utils/http-fetch-utils'

jest.mock('../../utils/http-fetch-utils', () => ({
  httpMidazFetch: jest.fn(),
  HTTP_METHODS: {
    GET: 'GET'
  }
}))

describe('MidazFetchSegmentByIdRepository', () => {
  let repository: MidazFetchSegmentByIdRepository
  let mockHttpFetchUtils: { httpMidazFetch: jest.Mock }

  beforeEach(() => {
    mockHttpFetchUtils = { httpMidazFetch: jest.fn() }
    repository = new MidazFetchSegmentByIdRepository(mockHttpFetchUtils as any)
    jest.clearAllMocks()
  })

  it('should fetch a segment by id successfully', async () => {
    const organizationId = '1'
    const ledgerId = '1'
    const segmentId = '1'
    const response: SegmentEntity = {
      id: '1',
      name: 'Test Segment',
      status: { code: 'ACTIVE', description: '' },
      metadata: {}
    }

    mockHttpFetchUtils.httpMidazFetch.mockResolvedValueOnce(response)

    const result = await repository.fetchById(
      organizationId,
      ledgerId,
      segmentId
    )

    expect(mockHttpFetchUtils.httpMidazFetch).toHaveBeenCalledWith({
      url: `${process.env.MIDAZ_BASE_PATH}/organizations/${organizationId}/ledgers/${ledgerId}/segments/${segmentId}`,
      method: HTTP_METHODS.GET
    })
    expect(result).toEqual(response)
  })

  it('should handle errors when fetching a segment by id', async () => {
    const organizationId = '1'
    const ledgerId = '1'
    const segmentId = '1'
    const error = new Error('Error occurred')

    mockHttpFetchUtils.httpMidazFetch.mockRejectedValueOnce(error)

    await expect(
      repository.fetchById(organizationId, ledgerId, segmentId)
    ).rejects.toThrow('Error occurred')

    expect(mockHttpFetchUtils.httpMidazFetch).toHaveBeenCalledWith({
      url: `${process.env.MIDAZ_BASE_PATH}/organizations/${organizationId}/ledgers/${ledgerId}/segments/${segmentId}`,
      method: HTTP_METHODS.GET
    })
  })
})
