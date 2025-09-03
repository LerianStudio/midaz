import { PaginationDto } from '../../dto/pagination-dto'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { TransactionRoutesMapper } from '../../mappers/transaction-routes-mapper'
import { TransactionRoutesEntity } from '@/core/domain/entities/transaction-routes-entity'
import {
  TransactionRoutesDto,
  type TransactionRoutesSearchParamDto
} from '../../dto/transaction-routes-dto'
import { TransactionRoutesRepository } from '@/core/domain/repositories/transaction-routes-repository'
import { inject, injectable } from 'inversify'
import { LogOperation } from '@/core/infrastructure/logger/decorators/log-operation'

export interface FetchAllTransactionRoutes {
  execute: (
    organizationId: string,
    ledgerId: string,
    query?: TransactionRoutesSearchParamDto
  ) => Promise<PaginationDto<TransactionRoutesDto>>
}

@injectable()
export class FetchAllTransactionRoutesUseCase
  implements FetchAllTransactionRoutes
{
  constructor(
    @inject(TransactionRoutesRepository)
    private readonly transactionRoutesRepository: TransactionRoutesRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    query?: TransactionRoutesSearchParamDto
  ): Promise<PaginationDto<TransactionRoutesDto>> {
    const transactionRoutesResult: PaginationEntity<TransactionRoutesEntity> =
      await this.transactionRoutesRepository.fetchAll(
        organizationId,
        ledgerId,
        query
      )

    // console.log('====================> teste leigo use case', transactionRoutesResult)

    return TransactionRoutesMapper.toPaginationResponseDto(
      transactionRoutesResult
    )
  }
}
