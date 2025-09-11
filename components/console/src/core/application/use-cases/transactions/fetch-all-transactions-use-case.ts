import { inject, injectable } from 'inversify'
import { TransactionRepository } from '@/core/domain/repositories/transaction-repository'
import { TransactionMapper } from '../../mappers/transaction-mapper'
import { CursorPaginationEntity } from '@/core/domain/entities/pagination-entity'
import {
  TransactionDto,
  type TransactionSearchDto
} from '../../dto/transaction-dto'
import { CursorPaginationDto } from '../../dto/pagination-dto'
import { LogOperation } from '@/core/infrastructure/logger/decorators/log-operation'

export interface FetchAllTransactions {
  execute: (
    organizationId: string,
    ledgerId: string,
    query?: TransactionSearchDto
  ) => Promise<CursorPaginationDto<TransactionDto>>
}

@injectable()
export class FetchAllTransactionsUseCase implements FetchAllTransactions {
  constructor(
    @inject(TransactionRepository)
    private readonly transactionRepository: TransactionRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    query?: TransactionSearchDto
  ): Promise<CursorPaginationDto<TransactionDto>> {
    const searchEntity = TransactionMapper.toSearchDomain(query || {})

    const transactionsResult: CursorPaginationEntity<any> =
      await this.transactionRepository.fetchAll(
        organizationId,
        ledgerId,
        searchEntity
      )

    return TransactionMapper.toCursorPaginationResponseDto(transactionsResult)
  }
}
