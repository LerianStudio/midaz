import { AssetEntity } from '@/core/domain/entities/asset-entity'
import { AssetResponseDto } from '../../dto/asset-response-dto'
import { CreateAssetRepository } from '@/core/domain/repositories/assets/create-asset-repository'
import type { CreateAssetDto } from '../../dto/create-asset-dto'
import { inject, injectable } from 'inversify'
import { AssetMapper } from '../../mappers/asset-mapper'
import { LogOperation } from '../../decorators/log-operation'

export interface CreateAsset {
  execute: (
    organizationId: string,
    ledgerId: string,
    asset: CreateAssetDto
  ) => Promise<AssetResponseDto>
}

@injectable()
export class CreateAssetUseCase implements CreateAsset {
  constructor(
    @inject(CreateAssetRepository)
    private readonly createAssetRepository: CreateAssetRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    asset: CreateAssetDto
  ): Promise<AssetResponseDto> {
    const assetEntity: AssetEntity = AssetMapper.toDomain(asset)

    const assetCreated = await this.createAssetRepository.create(
      organizationId,
      ledgerId,
      assetEntity
    )

    return AssetMapper.toResponseDto(assetCreated)
  }
}
