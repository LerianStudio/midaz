import { TransactionRoutesRepository } from '@/core/domain/repositories/transaction-routes-repository'
import { CursorPaginationDto } from '../../dto/pagination-dto'
import { inject, injectable } from 'inversify'
import { LogOperation } from '@/core/infrastructure/logger/decorators/log-operation'
import { TransactionRoutesMapper } from '../../mappers/transaction-routes-mapper'
import {
  TransactionRoutesDto,
  type TransactionRoutesSearchParamDto
} from '../../dto/transaction-routes-dto'
import { CursorPaginationEntity } from '@/core/domain/entities/pagination-entity'
import { TransactionRoutesEntity } from '@/core/domain/entities/transaction-routes-entity'

export interface FetchAllTransactionRoutesWithOperationRoutes {
  execute: (
    organizationId: string,
    ledgerId: string,
    query?: TransactionRoutesSearchParamDto
  ) => Promise<CursorPaginationDto<TransactionRoutesDto>>
}

@injectable()
export class FetchAllTransactionRoutesWithOperationRoutesUseCase implements FetchAllTransactionRoutesWithOperationRoutes {
  constructor(
    @inject(TransactionRoutesRepository)
    private readonly transactionRoutesRepository: TransactionRoutesRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    query?: TransactionRoutesSearchParamDto
  ): Promise<CursorPaginationDto<TransactionRoutesDto>> {
    const transactionRoutesResult: CursorPaginationEntity<TransactionRoutesEntity> =
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

    const enhancedResult: CursorPaginationEntity<TransactionRoutesEntity> = {
      ...transactionRoutesResult,
      items: transactionRoutesWithDetailedOperationRoutes
    }

    return TransactionRoutesMapper.toCursorPaginationResponseDto(enhancedResult)
  }
}
