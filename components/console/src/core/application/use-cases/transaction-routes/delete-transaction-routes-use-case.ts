import { TransactionRoutesRepository } from '@/core/domain/repositories/transaction-routes-repository'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface DeleteTransactionRoutes {
  execute: (
    organizationId: string,
    ledgerId: string,
    transactionRouteId: string
  ) => Promise<void>
}

@injectable()
export class DeleteTransactionRoutesUseCase implements DeleteTransactionRoutes {
  constructor(
    @inject(TransactionRoutesRepository)
    private readonly transactionRoutesRepository: TransactionRoutesRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    transactionRouteId: string
  ): Promise<void> {
    await this.transactionRoutesRepository.delete(
      organizationId,
      ledgerId,
      transactionRouteId
    )
  }
}
