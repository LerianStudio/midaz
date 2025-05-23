import { inject, injectable } from 'inversify'
import { FetchAllTransactionsRepository } from '@/core/domain/repositories/transactions/fetch-all-transactions-repository'
import { TransactionMapper } from '../../mappers/transaction-mapper'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { TransactionResponseDto } from '../../dto/transaction-dto'
import { LogOperation } from '../../decorators/log-operation'

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
    @inject(FetchAllTransactionsRepository)
    private readonly fetchAllTransactionsRepository: FetchAllTransactionsRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    limit: number,
    page: number
  ): Promise<PaginationEntity<TransactionResponseDto>> {
    const transactionsResult =
      await this.fetchAllTransactionsRepository.fetchAll(
        organizationId,
        ledgerId,
        limit,
        page
      )

    return TransactionMapper.toPaginatedResponseDto(transactionsResult)
  }
}
