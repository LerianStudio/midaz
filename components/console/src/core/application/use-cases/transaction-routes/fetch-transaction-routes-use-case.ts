import { TransactionRoutesRepository } from '@/core/domain/repositories/transaction-routes-repository'
import { TransactionRoutesDto } from '../../dto/transaction-routes-dto'
import { TransactionRoutesMapper } from '../../mappers/transaction-routes-mapper'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface FetchTransactionRoutesById {
  execute: (
    organizationId: string,
    ledgerId: string,
    transactionRouteId: string
  ) => Promise<TransactionRoutesDto>
}

@injectable()
export class FetchTransactionRoutesByIdUseCase implements FetchTransactionRoutesById {
  constructor(
    @inject(TransactionRoutesRepository)
    private readonly transactionRoutesRepository: TransactionRoutesRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    transactionRouteId: string
  ): Promise<TransactionRoutesDto> {
    const transactionRoute = await this.transactionRoutesRepository.fetchById(
      organizationId,
      ledgerId,
      transactionRouteId
    )

    return TransactionRoutesMapper.toDto(transactionRoute)
  }
}
