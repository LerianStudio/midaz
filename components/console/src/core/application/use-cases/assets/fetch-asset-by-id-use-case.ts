import { FetchAssetByIdRepository } from '@/core/domain/repositories/assets/fetch-asset-by-id-repository'
import { AssetResponseDto } from '../../dto/asset-response-dto'
import { AssetMapper } from '../../mappers/asset-mapper'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../decorators/log-operation'

export interface FetchAssetById {
  execute: (
    organizationId: string,
    ledgerId: string,
    assetId: string
  ) => Promise<AssetResponseDto>
}

@injectable()
export class FetchAssetByIdUseCase implements FetchAssetById {
  constructor(
    @inject(FetchAssetByIdRepository)
    private readonly fetchAssetByIdRepository: FetchAssetByIdRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    assetId: string
  ): Promise<AssetResponseDto> {
    const assetEntity = await this.fetchAssetByIdRepository.fetchById(
      organizationId,
      ledgerId,
      assetId
    )

    return AssetMapper.toResponseDto(assetEntity)
  }
}
