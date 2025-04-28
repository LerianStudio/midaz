import { inject, injectable } from 'inversify'
import { TransactionRepository } from '@/core/domain/repositories/transaction-repository'
import { TransactionMapper } from '../../mappers/transaction-mapper'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { TransactionResponseDto } from '../../dto/transaction-dto'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface FetchAllTransactions {
  execute: (
    organizationId: string,
    ledgerId: string,
    limit: number,
    page: number
  ) => Promise<PaginationEntity<TransactionResponseDto>>
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
    limit: number,
    page: number
  ): Promise<PaginationEntity<TransactionResponseDto>> {
    const transactionsResult = await this.transactionRepository.fetchAll(
      organizationId,
      ledgerId,
      limit,
      page
    )

    return TransactionMapper.toPaginatedResponseDto(transactionsResult)
  }
}
