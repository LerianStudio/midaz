import { AssetRepository } from '@/core/domain/repositories/asset-repository'
import { AssetResponseDto } from '../../dto/asset-dto'
import { AssetEntity } from '@/core/domain/entities/asset-entity'
import { AssetMapper } from '../../mappers/asset-mapper'
import { inject, injectable } from 'inversify'
import { CreateAssetDto, UpdateAssetDto } from '../../dto/asset-dto'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface UpdateAsset {
  execute: (
    organizationId: string,
    ledgerId: string,
    assetId: string,
    asset: Partial<UpdateAssetDto>
  ) => Promise<AssetResponseDto>
}

@injectable()
export class UpdateAssetUseCase implements UpdateAsset {
  constructor(
    @inject(AssetRepository)
    private readonly assetRepository: AssetRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    assetId: string,
    asset: Partial<UpdateAssetDto>
  ): Promise<AssetResponseDto> {
    const updateAssetEntity: Partial<AssetEntity> = AssetMapper.toDomain(
      asset as CreateAssetDto
    )

    const updatedAssetEntity = await this.assetRepository.update(
      organizationId,
      ledgerId,
      assetId,
      updateAssetEntity
    )

    return AssetMapper.toResponseDto(updatedAssetEntity)
  }
}
