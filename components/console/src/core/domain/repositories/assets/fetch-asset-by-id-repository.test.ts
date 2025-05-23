import { FetchAssetByIdRepository } from './fetch-asset-by-id-repository'
import { AssetEntity } from '../../entities/asset-entity'

describe('FetchAssetByIdRepository', () => {
  let fetchAssetByIdRepository: FetchAssetByIdRepository

  beforeEach(() => {
    fetchAssetByIdRepository = {
      fetchById: jest.fn()
    }
  })

  it('should fetch asset by id', async () => {
    const mockAsset: AssetEntity = {
      id: 'asset123',
      organizationId: 'org123',
      ledgerId: 'ledger123',
      name: 'Asset Name',
      type: 'Asset Type',
      code: 'Asset Code',
      status: { code: 'active', description: 'Active' },
      metadata: { key: 'value' },
      createdAt: new Date(),
      updatedAt: new Date(),
      deletedAt: null
    }

    ;(fetchAssetByIdRepository.fetchById as jest.Mock).mockResolvedValue(
      mockAsset
    )

    const organizationId = 'orgId'
    const ledgerId = 'ledgerId'
    const assetId = 'assetId'

    const result = await fetchAssetByIdRepository.fetchById(
      organizationId,
      ledgerId,
      assetId
    )

    expect(fetchAssetByIdRepository.fetchById).toHaveBeenCalledWith(
      organizationId,
      ledgerId,
      assetId
    )
    expect(result).toEqual(mockAsset)
  })

  it('should handle errors when fetching asset by id', async () => {
    const errorMessage = 'Error fetching asset'
    ;(fetchAssetByIdRepository.fetchById as jest.Mock).mockRejectedValue(
      new Error(errorMessage)
    )

    const organizationId = 'orgId'
    const ledgerId = 'ledgerId'
    const assetId = 'assetId'

    await expect(
      fetchAssetByIdRepository.fetchById(organizationId, ledgerId, assetId)
    ).rejects.toThrow(errorMessage)
  })
})
