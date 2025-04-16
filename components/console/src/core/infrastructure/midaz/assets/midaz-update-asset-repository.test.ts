import { MidazUpdateAssetRepository } from './midaz-update-asset-repository'
import { AssetEntity } from '@/core/domain/entities/asset-entity'
import { HTTP_METHODS } from '../../utils/http-fetch-utils'

jest.mock('../../utils/http-fetch-utils', () => ({
  httpMidazFetch: jest.fn(),
  HTTP_METHODS: { PATCH: 'PATCH' }
}))

describe('MidazUpdateAssetRepository', () => {
  let repository: MidazUpdateAssetRepository
  let mockHttpFetchUtils: { httpMidazFetch: jest.Mock }

  beforeEach(() => {
    mockHttpFetchUtils = { httpMidazFetch: jest.fn() }
    repository = new MidazUpdateAssetRepository(mockHttpFetchUtils as any)
    jest.clearAllMocks()
  })

  it('should update an asset successfully', async () => {
    const organizationId = '1'
    const ledgerId = '1'
    const assetId = '1'
    const assetData: Partial<AssetEntity> = { name: 'Updated Asset' }
    const response: AssetEntity = {
      id: assetId,
      name: 'Updated Asset',
      type: 'Asset Type',
      code: 'Asset Code',
      status: { code: 'active', description: 'Active' },
      metadata: { key: 'value' }
    }

    mockHttpFetchUtils.httpMidazFetch.mockResolvedValueOnce(response)

    const result = await repository.update(
      organizationId,
      ledgerId,
      assetId,
      assetData
    )

    expect(mockHttpFetchUtils.httpMidazFetch).toHaveBeenCalledWith({
      url: `${process.env.MIDAZ_BASE_PATH}/organizations/${organizationId}/ledgers/${ledgerId}/assets/${assetId}`,
      method: HTTP_METHODS.PATCH,
      body: JSON.stringify(assetData)
    })
    expect(result).toEqual(response)
  })

  it('should handle errors when updating an asset', async () => {
    const organizationId = '1'
    const ledgerId = '1'
    const assetId = '1'
    const assetData: Partial<AssetEntity> = { name: 'Updated Asset' }
    const error = new Error('Error occurred')

    mockHttpFetchUtils.httpMidazFetch.mockRejectedValueOnce(error)

    await expect(
      repository.update(organizationId, ledgerId, assetId, assetData)
    ).rejects.toThrow('Error occurred')

    expect(mockHttpFetchUtils.httpMidazFetch).toHaveBeenCalledWith({
      url: `${process.env.MIDAZ_BASE_PATH}/organizations/${organizationId}/ledgers/${ledgerId}/assets/${assetId}`,
      method: HTTP_METHODS.PATCH,
      body: JSON.stringify(assetData)
    })
  })
})
