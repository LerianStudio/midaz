import { CreateAssetRepository } from './create-asset-repository'
import { AssetEntity } from '../../entities/asset-entity'

describe('CreateAssetRepository', () => {
  let createAssetRepository: CreateAssetRepository

  beforeEach(() => {
    createAssetRepository = {
      create: jest.fn()
    }
  })

  it('should create an asset successfully', async () => {
    const organizationId = 'org123'
    const ledgerId = 'ledger123'
    const asset: AssetEntity = {
      id: 'asset123',
      name: 'Asset Name',
      type: 'Asset Type',
      code: 'Asset Code',
      status: { code: 'active', description: 'Active' },
      metadata: { key: 'value' }
    }

    ;(createAssetRepository.create as jest.Mock).mockResolvedValue(asset)

    const result = await createAssetRepository.create(
      organizationId,
      ledgerId,
      asset
    )

    expect(result).toEqual(asset)
    expect(createAssetRepository.create).toHaveBeenCalledWith(
      organizationId,
      ledgerId,
      asset
    )
  })

  it('should handle errors when creating an asset', async () => {
    const organizationId = 'org123'
    const ledgerId = 'ledger123'
    const asset: AssetEntity = {
      id: 'asset123',
      name: 'Asset Name',
      type: 'Asset Type',
      code: 'Asset Code',
      status: { code: 'active', description: 'Active' },
      metadata: { key: 'value' }
    }
    const error = new Error('Failed to create asset')

    ;(createAssetRepository.create as jest.Mock).mockRejectedValue(error)

    await expect(
      createAssetRepository.create(organizationId, ledgerId, asset)
    ).rejects.toThrow('Failed to create asset')
    expect(createAssetRepository.create).toHaveBeenCalledWith(
      organizationId,
      ledgerId,
      asset
    )
  })
})
