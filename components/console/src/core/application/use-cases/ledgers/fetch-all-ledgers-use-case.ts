import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { LedgerResponseDto } from '../../dto/ledger-response-dto'
import { FetchAllLedgersRepository } from '@/core/domain/repositories/ledgers/fetch-all-ledgers-repository'
import { LedgerEntity } from '@/core/domain/entities/ledger-entity'
import { PaginationDto } from '../../dto/pagination-dto'
import { inject, injectable } from 'inversify'
import { LedgerMapper } from '../../mappers/ledger-mapper'
import { LogOperation } from '../../decorators/log-operation'

export interface FetchAllLedgers {
  execute: (
    organizationId: string,
    limit: number,
    page: number
  ) => Promise<PaginationEntity<LedgerResponseDto>>
}

@injectable()
export class FetchAllLedgersUseCase implements FetchAllLedgers {
  constructor(
    @inject(FetchAllLedgersRepository)
    private readonly fetchAllLedgersRepository: FetchAllLedgersRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    limit: number,
    page: number
  ): Promise<PaginationDto<LedgerResponseDto>> {
    const ledgersResult: PaginationEntity<LedgerEntity> =
      await this.fetchAllLedgersRepository.fetchAll(organizationId, limit, page)

    return LedgerMapper.toPaginationResponseDto(ledgersResult)
  }
}
