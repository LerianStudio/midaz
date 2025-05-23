import { MidazDeleteAssetRepository } from './midaz-delete-asset-repository'
import { HTTP_METHODS } from '../../utils/http-fetch-utils'

jest.mock('../../utils/http-fetch-utils', () => ({
  httpMidazFetch: jest.fn(),
  HTTP_METHODS: {
    POST: 'DELETE'
  }
}))
describe('MidazDeleteAssetRepository', () => {
  let repository: MidazDeleteAssetRepository
  let mockHttpFetchUtils: { httpMidazFetch: jest.Mock }

  beforeEach(() => {
    mockHttpFetchUtils = { httpMidazFetch: jest.fn() }
    repository = new MidazDeleteAssetRepository(mockHttpFetchUtils as any)
    jest.clearAllMocks()
  })

  it('should delete an asset successfully', async () => {
    const organizationId = '1'
    const ledgerId = '1'
    const assetId = '1'

    mockHttpFetchUtils.httpMidazFetch.mockResolvedValueOnce(undefined)

    await repository.delete(organizationId, ledgerId, assetId)

    expect(mockHttpFetchUtils.httpMidazFetch).toHaveBeenCalledWith({
      url: `${process.env.MIDAZ_BASE_PATH}/organizations/${organizationId}/ledgers/${ledgerId}/assets/${assetId}`,
      method: HTTP_METHODS.DELETE
    })
  })

  it('should handle errors when deleting an asset', async () => {
    const organizationId = '1'
    const ledgerId = '1'
    const assetId = '1'
    const error = new Error('Error occurred')

    mockHttpFetchUtils.httpMidazFetch.mockRejectedValueOnce(error)

    await expect(
      repository.delete(organizationId, ledgerId, assetId)
    ).rejects.toThrow('Error occurred')

    expect(mockHttpFetchUtils.httpMidazFetch).toHaveBeenCalledWith({
      url: `${process.env.MIDAZ_BASE_PATH}/organizations/${organizationId}/ledgers/${ledgerId}/assets/${assetId}`,
      method: HTTP_METHODS.DELETE
    })
  })
})
