import { CreateAssetUseCase } from './create-asset-use-case'
import { CreateAssetRepository } from '@/core/domain/repositories/assets/create-asset-repository'
import { CreateAssetDto } from '../../dto/create-asset-dto'
import { AssetEntity } from '@/core/domain/entities/asset-entity'
import { AssetResponseDto } from '../../dto/asset-response-dto'
import { AssetMapper } from '../../mappers/asset-mapper'

jest.mock('../../mappers/asset-mapper')

describe('CreateAssetUseCase', () => {
  let createAssetRepository: jest.Mocked<CreateAssetRepository>
  let createAssetUseCase: CreateAssetUseCase

  beforeEach(() => {
    createAssetRepository = {
      create: jest.fn()
    } as jest.Mocked<CreateAssetRepository>
    createAssetUseCase = new CreateAssetUseCase(createAssetRepository)
  })

  it('should create an asset and return the response DTO', async () => {
    const organizationId = 'org123'
    const ledgerId = 'ledger123'
    const createAssetDto: CreateAssetDto = {
      name: 'Asset Name',
      code: 'asset123',
      type: 'asset',
      metadata: { key: 'value' },
      status: { code: 'active', description: 'Active' }
    }
    const assetEntity: AssetEntity = {
      id: 'asset123',
      organizationId: 'org123',
      ledgerId: 'ledger123',
      name: 'Asset Name',
      code: 'asset123',
      type: 'asset',
      metadata: { key: 'value' },
      status: { code: 'active', description: 'Active' },
      createdAt: new Date(),
      updatedAt: new Date(),
      deletedAt: null
    }
    const assetResponseDto: AssetResponseDto = {
      id: 'asset123',
      organizationId: 'org123',
      ledgerId: 'ledger123',
      name: 'Asset Name',
      code: 'asset123',
      type: 'asset',
      metadata: { key: 'value' },
      status: { code: 'active', description: 'Active' },
      createdAt: new Date(),
      updatedAt: new Date(),
      deletedAt: null
    }

    ;(AssetMapper.toDomain as jest.Mock).mockReturnValue(assetEntity)
    createAssetRepository.create.mockResolvedValue(assetEntity)
    ;(AssetMapper.toResponseDto as jest.Mock).mockReturnValue(assetResponseDto)

    const result = await createAssetUseCase.execute(
      organizationId,
      ledgerId,
      createAssetDto
    )

    expect(AssetMapper.toDomain).toHaveBeenCalledWith(createAssetDto)
    expect(createAssetRepository.create).toHaveBeenCalledWith(
      organizationId,
      ledgerId,
      assetEntity
    )
    expect(AssetMapper.toResponseDto).toHaveBeenCalledWith(assetEntity)
    expect(result).toEqual(assetResponseDto)
  })

  it('should throw an error if repository create fails', async () => {
    const organizationId = 'org123'
    const ledgerId = 'ledger123'
    const createAssetDto: CreateAssetDto = {
      name: 'Asset Name',
      code: 'asset123',
      type: 'asset',
      metadata: { key: 'value' },
      status: { code: 'active', description: 'Active' }
    }
    const assetEntity: AssetEntity = {
      id: 'asset123',
      organizationId: 'org123',
      ledgerId: 'ledger123',
      name: 'Asset Name',
      code: 'asset123',
      type: 'asset',
      metadata: { key: 'value' },
      status: { code: 'active', description: 'Active' },
      createdAt: new Date(),
      updatedAt: new Date(),
      deletedAt: null
    }

    ;(AssetMapper.toDomain as jest.Mock).mockReturnValue(assetEntity)
    createAssetRepository.create.mockRejectedValue(
      new Error('Repository create failed')
    )

    await expect(
      createAssetUseCase.execute(organizationId, ledgerId, createAssetDto)
    ).rejects.toThrow('Repository create failed')
  })
})
