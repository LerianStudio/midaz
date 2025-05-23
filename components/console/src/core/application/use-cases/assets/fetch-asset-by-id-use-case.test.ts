import { FetchAssetByIdUseCase } from './fetch-asset-by-id-use-case'
import { FetchAssetByIdRepository } from '@/core/domain/repositories/assets/fetch-asset-by-id-repository'
import { AssetResponseDto } from '../../dto/asset-response-dto'
import { AssetMapper } from '../../mappers/asset-mapper'
import { AssetEntity } from '@/core/domain/entities/asset-entity'

jest.mock('../../mappers/asset-mapper')

describe('FetchAssetByIdUseCase', () => {
  let fetchAssetByIdRepository: jest.Mocked<FetchAssetByIdRepository>
  let fetchAssetByIdUseCase: FetchAssetByIdUseCase

  beforeEach(() => {
    fetchAssetByIdRepository = {
      fetchById: jest.fn()
    }
    fetchAssetByIdUseCase = new FetchAssetByIdUseCase(fetchAssetByIdRepository)
  })

  it('should fetch asset by id and return the asset DTO', async () => {
    const organizationId = 'org123'
    const ledgerId = 'ledger123'
    const assetId = 'asset123'
    const assetEntity: AssetEntity = {
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
    const assetDto: AssetResponseDto = {
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

    fetchAssetByIdRepository.fetchById.mockResolvedValue(assetEntity)
    ;(AssetMapper.toResponseDto as jest.Mock).mockReturnValue(assetDto)

    const result = await fetchAssetByIdUseCase.execute(
      organizationId,
      ledgerId,
      assetId
    )

    expect(fetchAssetByIdRepository.fetchById).toHaveBeenCalledWith(
      organizationId,
      ledgerId,
      assetId
    )
    expect(AssetMapper.toResponseDto).toHaveBeenCalledWith(assetEntity)
    expect(result).toEqual(assetDto)
  })

  it('should throw an error if fetchById fails', async () => {
    const organizationId = 'org123'
    const ledgerId = 'ledger123'
    const assetId = 'asset123'

    fetchAssetByIdRepository.fetchById.mockRejectedValue(
      new Error('Fetch failed')
    )

    await expect(
      fetchAssetByIdUseCase.execute(organizationId, ledgerId, assetId)
    ).rejects.toThrow('Fetch failed')
  })
})
