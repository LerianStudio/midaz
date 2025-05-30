import { AssetEntity } from '@/core/domain/entities/asset-entity'
import { AssetRepository } from '@/core/domain/repositories/asset-repository'
import type { CreateAssetDto, AssetDto } from '../../dto/asset-dto'
import { inject, injectable } from 'inversify'
import { AssetMapper } from '../../mappers/asset-mapper'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface CreateAsset {
  execute: (
    organizationId: string,
    ledgerId: string,
    asset: CreateAssetDto
  ) => Promise<AssetDto>
}

@injectable()
export class CreateAssetUseCase implements CreateAsset {
  constructor(
    @inject(AssetRepository)
    private readonly assetRepository: AssetRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    asset: CreateAssetDto
  ): Promise<AssetDto> {
    const assetEntity: AssetEntity = AssetMapper.toDomain(asset)

    const assetCreated = await this.assetRepository.create(
      organizationId,
      ledgerId,
      assetEntity
    )

    return AssetMapper.toResponseDto(assetCreated)
  }
}
