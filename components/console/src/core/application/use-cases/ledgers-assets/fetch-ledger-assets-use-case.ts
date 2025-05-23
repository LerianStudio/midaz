import { FetchAllAssetsRepository } from '@/core/domain/repositories/assets/fetch-all-assets-repository'
import { FetchAllLedgersRepository } from '@/core/domain/repositories/ledgers/fetch-all-ledgers-repository'
import { LedgersViewResponseDTO } from '../../dto/ledgers-view-dto'
import { PaginationDto } from '../../dto/pagination-dto'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { LedgerEntity } from '@/core/domain/entities/ledger-entity'
import { AssetEntity } from '@/core/domain/entities/asset-entity'
import { inject, injectable } from 'inversify'
import { AssetMapper } from '../../mappers/asset-mapper'
import { LogOperation } from '../../decorators/log-operation'

export interface FetchAllLedgersAssets {
  execute: (
    organizationId: string,
    limit: number,
    page: number
  ) => Promise<PaginationDto<LedgersViewResponseDTO>>
}

@injectable()
export class FetchAllLedgersAssetsUseCase implements FetchAllLedgersAssets {
  constructor(
    @inject(FetchAllLedgersRepository)
    private readonly fetchAllLedgersRepository: FetchAllLedgersRepository,
    @inject(FetchAllAssetsRepository)
    private readonly fetchAllAssetsRepository: FetchAllAssetsRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    limit: number,
    page: number
  ): Promise<PaginationDto<LedgersViewResponseDTO>> {
    const ledgersResult: PaginationEntity<LedgerEntity> =
      await this.fetchAllLedgersRepository.fetchAll(organizationId, limit, page)

    let ledgersAssetResponseDTO: PaginationDto<LedgersViewResponseDTO> = {
      items: [],
      limit: ledgersResult.limit,
      page: ledgersResult.page
    }

    const ledgerItems = ledgersResult.items || []

    ledgersAssetResponseDTO.items = await Promise.all(
      ledgerItems.map(async (ledger) => {
        const assetsResult: PaginationEntity<AssetEntity> =
          await this.fetchAllAssetsRepository.fetchAll(
            organizationId,
            ledger.id!,
            limit,
            page
          )

        const ledgerAssets: LedgersViewResponseDTO = {
          id: ledger.id!,
          organizationId: ledger.organizationId!,
          name: ledger.name!,
          status: {
            code: ledger.status!.code!,
            description: ledger.status!.description!
          },
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
