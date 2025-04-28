import { UpdateAssetUseCase } from './update-asset-use-case'
import { UpdateAssetRepository } from '@/core/domain/repositories/assets/update-asset-repository'
import { AssetResponseDto } from '../../dto/asset-response-dto'
import { UpdateAssetDto } from '../../dto/update-asset-dto'
import { AssetEntity } from '@/core/domain/entities/asset-entity'
import { AssetMapper } from '../../mappers/asset-mapper'

jest.mock('../../mappers/asset-mapper')

describe('UpdateAssetUseCase', () => {
  let updateAssetUseCase: UpdateAssetUseCase
  let updateAssetRepository: jest.Mocked<UpdateAssetRepository>

  beforeEach(() => {
    updateAssetRepository = {
      update: jest.fn()
    } as jest.Mocked<UpdateAssetRepository>
    updateAssetUseCase = new UpdateAssetUseCase(updateAssetRepository)
  })

  it('should update an asset and return the updated asset response DTO', async () => {
    const organizationId = 'org123'
    const ledgerId = 'ledger123'
    const assetId = 'asset123'
    const updateAssetDto: Partial<UpdateAssetDto> = { name: 'Updated Asset' }
    const updateAssetEntity: Partial<AssetEntity> = { name: 'Updated Asset' }
    const updatedAssetEntity: AssetEntity = {
      id: 'asset123',
      name: 'Updated Asset'
    } as AssetEntity
    const assetResponseDto: AssetResponseDto = {
      id: 'asset123',
      name: 'Updated Asset'
    } as AssetResponseDto

    ;(AssetMapper.toDomain as jest.Mock).mockReturnValue(updateAssetEntity)
    updateAssetRepository.update.mockResolvedValue(updatedAssetEntity)
    ;(AssetMapper.toResponseDto as jest.Mock).mockReturnValue(assetResponseDto)

    const result = await updateAssetUseCase.execute(
      organizationId,
      ledgerId,
      assetId,
      updateAssetDto
    )

    expect(AssetMapper.toDomain).toHaveBeenCalledWith(updateAssetDto)
    expect(updateAssetRepository.update).toHaveBeenCalledWith(
      organizationId,
      ledgerId,
      assetId,
      updateAssetEntity
    )
    expect(AssetMapper.toResponseDto).toHaveBeenCalledWith(updatedAssetEntity)
    expect(result).toEqual(assetResponseDto)
  })

  it('should throw an error if the update fails', async () => {
    const organizationId = 'org123'
    const ledgerId = 'ledger123'
    const assetId = 'asset123'
    const updateAssetDto: Partial<UpdateAssetDto> = { name: 'Updated Asset' }
    const updateAssetEntity: Partial<AssetEntity> = { name: 'Updated Asset' }

    ;(AssetMapper.toDomain as jest.Mock).mockReturnValue(updateAssetEntity)
    updateAssetRepository.update.mockRejectedValue(new Error('Update failed'))

    await expect(
      updateAssetUseCase.execute(
        organizationId,
        ledgerId,
        assetId,
        updateAssetDto
      )
    ).rejects.toThrow('Update failed')
  })
})
