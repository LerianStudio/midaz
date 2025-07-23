import { inject, injectable } from 'inversify'
import { TransactionRepository } from '@/core/domain/repositories/transaction-repository'
import { TransactionMapper } from '../../mappers/transaction-mapper'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import {
  TransactionDto,
  type TransactionSearchDto
} from '../../dto/transaction-dto'
import { LogOperation } from '@/core/infrastructure/logger/decorators/log-operation'

export interface FetchAllTransactions {
  execute: (
    organizationId: string,
    ledgerId: string,
    filters: TransactionSearchDto
  ) => Promise<PaginationEntity<TransactionDto>>
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
    filters: TransactionSearchDto
  ): Promise<PaginationEntity<TransactionDto>> {
    const transactionsResult = await this.transactionRepository.fetchAll(
      organizationId,
      ledgerId,
      filters
    )

    return TransactionMapper.toPaginatedResponseDto(transactionsResult)
  }
}
