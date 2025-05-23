import { MidazDeleteSegmentRepository } from './midaz-delete-segment-repository'
import { HTTP_METHODS } from '../../utils/http-fetch-utils'

jest.mock('../../utils/http-fetch-utils', () => ({
  httpMidazFetch: jest.fn(),
  HTTP_METHODS: {
    DELETE: 'DELETE'
  }
}))

describe('MidazDeleteSegmentRepository', () => {
  let repository: MidazDeleteSegmentRepository
  let mockHttpFetchUtils: { httpMidazFetch: jest.Mock }

  beforeEach(() => {
    mockHttpFetchUtils = { httpMidazFetch: jest.fn() }
    repository = new MidazDeleteSegmentRepository(mockHttpFetchUtils as any)
    jest.clearAllMocks()
  })

  it('should delete a segment successfully', async () => {
    const organizationId = '1'
    const ledgerId = '1'
    const segmentId = '1'

    mockHttpFetchUtils.httpMidazFetch.mockResolvedValueOnce(undefined)

    await repository.delete(organizationId, ledgerId, segmentId)

    expect(mockHttpFetchUtils.httpMidazFetch).toHaveBeenCalledWith({
      url: `${process.env.MIDAZ_BASE_PATH}/organizations/${organizationId}/ledgers/${ledgerId}/segments/${segmentId}`,
      method: HTTP_METHODS.DELETE
    })
  })

  it('should handle errors when deleting a segment', async () => {
    const organizationId = '1'
    const ledgerId = '1'
    const segmentId = '1'
    const error = new Error('Error occurred')

    mockHttpFetchUtils.httpMidazFetch.mockRejectedValueOnce(error)

    await expect(
      repository.delete(organizationId, ledgerId, segmentId)
    ).rejects.toThrow('Error occurred')

    expect(mockHttpFetchUtils.httpMidazFetch).toHaveBeenCalledWith({
      url: `${process.env.MIDAZ_BASE_PATH}/organizations/${organizationId}/ledgers/${ledgerId}/segments/${segmentId}`,
      method: HTTP_METHODS.DELETE
    })
  })
})
