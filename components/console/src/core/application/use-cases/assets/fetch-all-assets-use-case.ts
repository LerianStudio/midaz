import { FetchAllAssetsRepository } from '@/core/domain/repositories/assets/fetch-all-assets-repository'
import { AssetResponseDto } from '../../dto/asset-response-dto'
import { PaginationDto } from '../../dto/pagination-dto'
import { AssetEntity } from '@/core/domain/entities/asset-entity'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { AssetMapper } from '../../mappers/asset-mapper'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../decorators/log-operation'

export interface FetchAllAssets {
  execute: (
    organizationId: string,
    ledgerId: string,
    limit: number,
    page: number,
    type?: string,
    code?: string,
    metadata?: Record<string, string>
  ) => Promise<PaginationDto<AssetResponseDto>>
}

@injectable()
export class FetchAllAssetsUseCase implements FetchAllAssets {
  constructor(
    @inject(FetchAllAssetsRepository)
    private readonly fetchAllAssetsRepository: FetchAllAssetsRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    limit: number,
    page: number,
    type?: string,
    code?: string,
    metadata?: Record<string, string>
  ): Promise<PaginationDto<AssetResponseDto>> {
    const assetsResult: PaginationEntity<AssetEntity> =
      await this.fetchAllAssetsRepository.fetchAll(
        organizationId,
        ledgerId,
        limit,
        page,
        type,
        code,
        metadata
      )

    return AssetMapper.toPaginationResponseDto(assetsResult)
  }
}
