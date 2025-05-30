import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { LedgerDto } from '../../dto/ledger-dto'
import { LedgerRepository } from '@/core/domain/repositories/ledger-repository'
import { LedgerEntity } from '@/core/domain/entities/ledger-entity'
import { PaginationDto } from '../../dto/pagination-dto'
import { inject, injectable } from 'inversify'
import { LedgerMapper } from '../../mappers/ledger-mapper'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface FetchAllLedgers {
  execute: (
    organizationId: string,
    limit: number,
    page: number
  ) => Promise<PaginationEntity<LedgerDto>>
}

@injectable()
export class FetchAllLedgersUseCase implements FetchAllLedgers {
  constructor(
    @inject(LedgerRepository)
    private readonly ledgerRepository: LedgerRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    limit: number,
    page: number
  ): Promise<PaginationDto<LedgerDto>> {
    const ledgersResult: PaginationEntity<LedgerEntity> =
      await this.ledgerRepository.fetchAll(organizationId, limit, page)

    return LedgerMapper.toPaginationResponseDto(ledgersResult)
  }
}
