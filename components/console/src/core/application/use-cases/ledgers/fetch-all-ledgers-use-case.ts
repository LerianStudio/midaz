import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { type LedgerDto, type LedgerSearchParamDto } from '../../dto/ledger-dto'
import { LedgerRepository } from '@/core/domain/repositories/ledger-repository'
import { PaginationDto } from '../../dto/pagination-dto'
import { inject, injectable } from 'inversify'
import { LedgerMapper } from '../../mappers/ledger-mapper'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface FetchAllLedgers {
  execute: (
    organizationId: string,
    filters: LedgerSearchParamDto
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
    filters: LedgerSearchParamDto
  ): Promise<PaginationDto<LedgerDto>> {
    const ledgersResult = await this.ledgerRepository.fetchAll(
      organizationId,
      filters
    )

    return LedgerMapper.toPaginationResponseDto(ledgersResult)
  }
}
