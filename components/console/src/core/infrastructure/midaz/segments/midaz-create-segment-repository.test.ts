import { MidazCreateSegmentRepository } from './midaz-create-segment-repository'
import { SegmentEntity } from '@/core/domain/entities/segment-entity'
import { HTTP_METHODS } from '../../utils/http-fetch-utils'

jest.mock('../../utils/http-fetch-utils', () => ({
  httpMidazFetch: jest.fn(),
  HTTP_METHODS: {
    POST: 'POST'
  }
}))

describe('MidazCreateSegmentRepository', () => {
  let repository: MidazCreateSegmentRepository
  let mockHttpFetchUtils: { httpMidazFetch: jest.Mock }

  beforeEach(() => {
    mockHttpFetchUtils = { httpMidazFetch: jest.fn() }
    repository = new MidazCreateSegmentRepository(mockHttpFetchUtils as any)
    jest.clearAllMocks()
  })

  it('should create a segment successfully', async () => {
    const organizationId = '1'
    const ledgerId = '1'
    const segment: SegmentEntity = {
      id: '1',
      name: 'Test Segment',
      status: { code: 'ACTIVE', description: '' },
      metadata: {}
    }
    const response: SegmentEntity = { ...segment }

    mockHttpFetchUtils.httpMidazFetch.mockResolvedValueOnce(response)

    const result = await repository.create(organizationId, ledgerId, segment)

    expect(mockHttpFetchUtils.httpMidazFetch).toHaveBeenCalledWith({
      url: `${process.env.MIDAZ_BASE_PATH}/organizations/${organizationId}/ledgers/${ledgerId}/segments`,
      method: HTTP_METHODS.POST,
      body: JSON.stringify(segment)
    })
    expect(result).toEqual(response)
  })

  it('should handle errors when creating a segment', async () => {
    const organizationId = '1'
    const ledgerId = '1'
    const segment: SegmentEntity = {
      id: '1',
      name: 'Test Segment',
      status: { code: 'ACTIVE', description: '' },
      metadata: {}
    }
    const error = new Error('Error occurred')

    mockHttpFetchUtils.httpMidazFetch.mockRejectedValueOnce(error)

    await expect(
      repository.create(organizationId, ledgerId, segment)
    ).rejects.toThrow('Error occurred')

    expect(mockHttpFetchUtils.httpMidazFetch).toHaveBeenCalledWith({
      url: `${process.env.MIDAZ_BASE_PATH}/organizations/${organizationId}/ledgers/${ledgerId}/segments`,
      method: HTTP_METHODS.POST,
      body: JSON.stringify(segment)
    })
  })
})
