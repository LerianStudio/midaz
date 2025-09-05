import { TransactionRoutesRepository } from '@/core/domain/repositories/transaction-routes-repository'
import { PaginationDto } from '../../dto/pagination-dto'
import { inject, injectable } from 'inversify'
import { LogOperation } from '@/core/infrastructure/logger/decorators/log-operation'
import { TransactionRoutesMapper } from '../../mappers/transaction-routes-mapper'
import {
  TransactionRoutesDto,
  type TransactionRoutesSearchParamDto
} from '../../dto/transaction-routes-dto'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { TransactionRoutesEntity } from '@/core/domain/entities/transaction-routes-entity'

export interface FetchAllTransactionRoutesWithOperationRoutes {
  execute: (
    organizationId: string,
    ledgerId: string,
    query?: TransactionRoutesSearchParamDto
  ) => Promise<PaginationDto<TransactionRoutesDto>>
}

@injectable()
export class FetchAllTransactionRoutesWithOperationRoutesUseCase
  implements FetchAllTransactionRoutesWithOperationRoutes
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

    const transactionRoutesWithDetailedOperationRoutes: TransactionRoutesEntity[] =
      await Promise.all(
        transactionRoutesResult.items.map(async (transactionRoute) => {
          try {
            const detailedTransactionRoute =
              await this.transactionRoutesRepository.fetchById(
                organizationId,
                ledgerId,
                transactionRoute.id!
              )

            return detailedTransactionRoute
          } catch (error) {
            console.warn(
              `Failed to fetch details for transaction route ${transactionRoute.id}:`,
              error
            )
            return transactionRoute
          }
        })
      )

    const enhancedResult: PaginationEntity<TransactionRoutesEntity> = {
      ...transactionRoutesResult,
      items: transactionRoutesWithDetailedOperationRoutes
    }

    return TransactionRoutesMapper.toPaginationResponseDto(enhancedResult)
  }
}
