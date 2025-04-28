import { MidazUpdateSegmentRepository } from './midaz-update-segment-repository'
import { SegmentEntity } from '@/core/domain/entities/segment-entity'
import { HTTP_METHODS } from '../../utils/http-fetch-utils'

jest.mock('../../utils/http-fetch-utils', () => ({
  httpMidazFetch: jest.fn(),
  HTTP_METHODS: {
    PATCH: 'PATCH'
  }
}))

describe('MidazUpdateSegmentRepository', () => {
  let repository: MidazUpdateSegmentRepository
  let mockHttpFetchUtils: { httpMidazFetch: jest.Mock }

  beforeEach(() => {
    mockHttpFetchUtils = { httpMidazFetch: jest.fn() }
    repository = new MidazUpdateSegmentRepository(mockHttpFetchUtils as any)
    jest.clearAllMocks()
  })

  it('should update a segment successfully', async () => {
    const organizationId = '1'
    const ledgerId = '1'
    const segmentId = '1'
    const segmentData: Partial<SegmentEntity> = { name: 'Updated Segment' }
    const response: SegmentEntity = {
      id: segmentId,
      name: 'Updated Segment',
      status: { code: 'ACTIVE', description: '' },
      metadata: {}
    }

    mockHttpFetchUtils.httpMidazFetch.mockResolvedValueOnce(response)

    const result = await repository.update(
      organizationId,
      ledgerId,
      segmentId,
      segmentData
    )

    expect(mockHttpFetchUtils.httpMidazFetch).toHaveBeenCalledWith({
      url: `${process.env.MIDAZ_BASE_PATH}/organizations/${organizationId}/ledgers/${ledgerId}/segments/${segmentId}`,
      method: HTTP_METHODS.PATCH,
      body: JSON.stringify(segmentData)
    })
    expect(result).toEqual(response)
  })

  it('should handle errors when updating a segment', async () => {
    const organizationId = '1'
    const ledgerId = '1'
    const segmentId = '1'
    const segmentData: Partial<SegmentEntity> = { name: 'Updated Segment' }
    const error = new Error('Error occurred')

    mockHttpFetchUtils.httpMidazFetch.mockRejectedValueOnce(error)

    await expect(
      repository.update(organizationId, ledgerId, segmentId, segmentData)
    ).rejects.toThrow('Error occurred')

    expect(mockHttpFetchUtils.httpMidazFetch).toHaveBeenCalledWith({
      url: `${process.env.MIDAZ_BASE_PATH}/organizations/${organizationId}/ledgers/${ledgerId}/segments/${segmentId}`,
      method: HTTP_METHODS.PATCH,
      body: JSON.stringify(segmentData)
    })
  })
})
