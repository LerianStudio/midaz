import { AssetRepository } from '@/core/domain/repositories/asset-repository'
import { LedgerRepository } from '@/core/domain/repositories/ledger-repository'
import { type LedgerDto, type LedgerSearchParamDto } from '../../dto/ledger-dto'
import { PaginationDto } from '../../dto/pagination-dto'
import { AssetEntity } from '@/core/domain/entities/asset-entity'
import { inject, injectable } from 'inversify'
import { AssetMapper } from '../../mappers/asset-mapper'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'

export interface FetchAllLedgersAssets {
  execute: (
    organizationId: string,
    filters: LedgerSearchParamDto
  ) => Promise<PaginationDto<LedgerDto>>
}

@injectable()
export class FetchAllLedgersAssetsUseCase implements FetchAllLedgersAssets {
  constructor(
    @inject(LedgerRepository)
    private readonly ledgerRepository: LedgerRepository,
    @inject(AssetRepository)
    private readonly assetRepository: AssetRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    filters: LedgerSearchParamDto
  ): Promise<PaginationDto<LedgerDto>> {
    const ledgersResult = await this.ledgerRepository.fetchAll(
      organizationId,
      filters
    )

    let ledgersAssetResponseDTO: PaginationDto<LedgerDto> = {
      items: [],
      limit: ledgersResult.limit,
      page: ledgersResult.page
    }

    const ledgerItems = ledgersResult.items || []

    ledgersAssetResponseDTO.items = await Promise.all(
      ledgerItems.map(async (ledger) => {
        const assetsResult: PaginationEntity<AssetEntity> =
          await this.assetRepository.fetchAll(
            organizationId,
            ledger.id!,
            100,
            1
          )

        const ledgerAssets: LedgerDto = {
          id: ledger.id!,
          organizationId: ledger.organizationId!,
          name: ledger.name!,
          metadata: ledger.metadata!,
          createdAt: ledger.createdAt!,
          updatedAt: ledger.updatedAt!,
          deletedAt: ledger.deletedAt!,
          assets: assetsResult.items
            ? assetsResult.items.map(AssetMapper.toResponseDto)
            : []
        }

        return ledgerAssets
      })
    )

    return ledgersAssetResponseDTO
  }
}
