import { UpdateAssetRepository } from '@/core/domain/repositories/assets/update-asset-repository'
import { AssetResponseDto } from '../../dto/asset-response-dto'
import { UpdateAssetDto } from '../../dto/update-asset-dto'
import { AssetEntity } from '@/core/domain/entities/asset-entity'
import { AssetMapper } from '../../mappers/asset-mapper'
import { inject, injectable } from 'inversify'
import { CreateAssetDto } from '../../dto/create-asset-dto'
import { LogOperation } from '../../decorators/log-operation'

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
    @inject(UpdateAssetRepository)
    private readonly updateAssetRepository: UpdateAssetRepository
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

    const updatedAssetEntity = await this.updateAssetRepository.update(
      organizationId,
      ledgerId,
      assetId,
      updateAssetEntity
    )

    return AssetMapper.toResponseDto(updatedAssetEntity)
  }
}
