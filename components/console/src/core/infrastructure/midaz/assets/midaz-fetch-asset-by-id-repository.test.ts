import { MidazFetchAssetByIdRepository } from './midaz-fetch-asset-by-id-repository'
import { AssetEntity } from '@/core/domain/entities/asset-entity'
import { HTTP_METHODS } from '../../utils/http-fetch-utils'

jest.mock('../../utils/http-fetch-utils', () => ({
  httpMidazFetch: jest.fn(),
  HTTP_METHODS: { GET: 'GET' }
}))

describe('MidazFetchAssetByIdRepository', () => {
  let repository: MidazFetchAssetByIdRepository
  let mockHttpFetchUtils: { httpMidazFetch: jest.Mock }

  beforeEach(() => {
    mockHttpFetchUtils = { httpMidazFetch: jest.fn() }
    repository = new MidazFetchAssetByIdRepository(mockHttpFetchUtils as any)
    jest.clearAllMocks()
  })

  it('should fetch an asset by id successfully', async () => {
    const organizationId = '1'
    const ledgerId = '1'
    const assetId = '1'
    const response: AssetEntity = {
      id: 'asset123',
      name: 'Asset Name',
      type: 'Asset Type',
      code: 'Asset Code',
      status: { code: 'active', description: 'Active' },
      metadata: { key: 'value' }
    }

    mockHttpFetchUtils.httpMidazFetch.mockResolvedValueOnce(response)

    const result = await repository.fetchById(organizationId, ledgerId, assetId)

    expect(mockHttpFetchUtils.httpMidazFetch).toHaveBeenCalledWith({
      url: `${process.env.MIDAZ_BASE_PATH}/organizations/${organizationId}/ledgers/${ledgerId}/assets/${assetId}`,
      method: HTTP_METHODS.GET
    })
    expect(result).toEqual(response)
  })

  it('should handle errors when fetching an asset by id', async () => {
    const organizationId = '1'
    const ledgerId = '1'
    const assetId = '1'
    const error = new Error('Error occurred')

    mockHttpFetchUtils.httpMidazFetch.mockRejectedValueOnce(error)

    await expect(
      repository.fetchById(organizationId, ledgerId, assetId)
    ).rejects.toThrow('Error occurred')

    expect(mockHttpFetchUtils.httpMidazFetch).toHaveBeenCalledWith({
      url: `${process.env.MIDAZ_BASE_PATH}/organizations/${organizationId}/ledgers/${ledgerId}/assets/${assetId}`,
      method: HTTP_METHODS.GET
    })
  })
})
