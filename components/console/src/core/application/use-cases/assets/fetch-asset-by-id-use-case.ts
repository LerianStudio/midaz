import { AssetRepository } from '@/core/domain/repositories/asset-repository'
import { AssetDto } from '../../dto/asset-dto'
import { AssetMapper } from '../../mappers/asset-mapper'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface FetchAssetById {
  execute: (
    organizationId: string,
    ledgerId: string,
    assetId: string
  ) => Promise<AssetDto>
}

@injectable()
export class FetchAssetByIdUseCase implements FetchAssetById {
  constructor(
    @inject(AssetRepository)
    private readonly assetRepository: AssetRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    assetId: string
  ): Promise<AssetDto> {
    const assetEntity = await this.assetRepository.fetchById(
      organizationId,
      ledgerId,
      assetId
    )

    return AssetMapper.toResponseDto(assetEntity)
  }
}
