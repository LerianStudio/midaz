import { UpdateAssetRepository } from './update-asset-repository'
import { AssetEntity } from '../../entities/asset-entity'

describe('UpdateAssetRepository', () => {
  let updateAssetRepository: UpdateAssetRepository

  beforeEach(() => {
    updateAssetRepository = {
      update: jest.fn()
    }
  })

  it('should update an asset and return the updated asset', async () => {
    const organizationId = 'org123'
    const ledgerId = 'ledger123'
    const assetId = 'asset123'
    const asset: Partial<AssetEntity> = { name: 'Updated Asset' }
    const updatedAsset: AssetEntity = {
      id: assetId,
      organizationId: organizationId,
      ledgerId: ledgerId,
      name: 'Updated Asset',
      type: 'Asset Type',
      code: 'Asset Code',
      status: { code: 'active', description: 'Active' },
      metadata: { key: 'value' },
      createdAt: new Date(),
      updatedAt: new Date(),
      deletedAt: null
    }

    ;(updateAssetRepository.update as jest.Mock).mockResolvedValue(updatedAsset)

    const result = await updateAssetRepository.update(
      organizationId,
      ledgerId,
      assetId,
      asset
    )

    expect(updateAssetRepository.update).toHaveBeenCalledWith(
      organizationId,
      ledgerId,
      assetId,
      asset
    )
    expect(result).toEqual(updatedAsset)
  })

  it('should throw an error if update fails', async () => {
    const organizationId = 'org123'
    const ledgerId = 'ledger123'
    const assetId = 'asset123'
    const asset: Partial<AssetEntity> = { name: 'Updated Asset' }
    const errorMessage = 'Update failed'

    ;(updateAssetRepository.update as jest.Mock).mockRejectedValue(
      new Error(errorMessage)
    )

    await expect(
      updateAssetRepository.update(organizationId, ledgerId, assetId, asset)
    ).rejects.toThrow(errorMessage)
  })
})
