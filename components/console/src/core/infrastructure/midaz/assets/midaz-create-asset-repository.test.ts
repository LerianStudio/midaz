import { MidazCreateAssetRepository } from './midaz-create-asset-repository'
import { AssetEntity } from '@/core/domain/entities/asset-entity'
import { HTTP_METHODS } from '../../utils/http-fetch-utils'

jest.mock('../../utils/http-fetch-utils', () => ({
  httpMidazFetch: jest.fn(),
  HTTP_METHODS: {
    POST: 'POST'
  }
}))

describe('MidazCreateAssetRepository', () => {
  let repository: MidazCreateAssetRepository
  let mockHttpFetchUtils: { httpMidazFetch: jest.Mock }

  beforeEach(() => {
    mockHttpFetchUtils = { httpMidazFetch: jest.fn() }
    repository = new MidazCreateAssetRepository(mockHttpFetchUtils as any)
    jest.clearAllMocks()
  })

  it('should create an asset successfully', async () => {
    const organizationId = '1'
    const ledgerId = '1'
    const asset: AssetEntity = {
      id: 'asset123',
      name: 'Asset Name',
      type: 'Asset Type',
      code: 'Asset Code',
      status: { code: 'active', description: 'Active' },
      metadata: { key: 'value' }
    }
    const response: AssetEntity = { ...asset }

    mockHttpFetchUtils.httpMidazFetch.mockResolvedValueOnce(response)

    const result = await repository.create(organizationId, ledgerId, asset)

    expect(mockHttpFetchUtils.httpMidazFetch).toHaveBeenCalledWith({
      url: `${process.env.MIDAZ_BASE_PATH}/organizations/${organizationId}/ledgers/${ledgerId}/assets`,
      method: HTTP_METHODS.POST,
      body: JSON.stringify(asset)
    })
    expect(result).toEqual(response)
  })

  it('should handle errors when creating an asset', async () => {
    const organizationId = '1'
    const ledgerId = '1'
    const asset: AssetEntity = {
      id: 'asset123',
      name: 'Asset Name',
      type: 'Asset Type',
      code: 'Asset Code',
      status: { code: 'active', description: 'Active' },
      metadata: { key: 'value' }
    }
    const error = new Error('Error occurred')

    mockHttpFetchUtils.httpMidazFetch.mockRejectedValueOnce(error)

    await expect(
      repository.create(organizationId, ledgerId, asset)
    ).rejects.toThrow('Error occurred')

    expect(mockHttpFetchUtils.httpMidazFetch).toHaveBeenCalledWith({
      url: `${process.env.MIDAZ_BASE_PATH}/organizations/${organizationId}/ledgers/${ledgerId}/assets`,
      method: HTTP_METHODS.POST,
      body: JSON.stringify(asset)
    })
  })
})
