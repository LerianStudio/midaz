import { TransactionRoutesRepository } from '@/core/domain/repositories/transaction-routes-repository'
import { TransactionRoutesEntity } from '@/core/domain/entities/transaction-routes-entity'
import { TransactionRoutesMapper } from '../../mappers/transaction-routes-mapper'
import { inject, injectable } from 'inversify'
import { LogOperation } from '@/core/infrastructure/logger/decorators/log-operation'
import type {
  CreateTransactionRoutesDto,
  TransactionRoutesDto
} from '../../dto/transaction-routes-dto'

export interface CreateTransactionRoutes {
  execute: (
    organizationId: string,
    ledgerId: string,
    transactionRoute: CreateTransactionRoutesDto
  ) => Promise<TransactionRoutesDto>
}

@injectable()
export class CreateTransactionRoutesUseCase implements CreateTransactionRoutes {
  constructor(
    @inject(TransactionRoutesRepository)
    private readonly transactionRoutesRepository: TransactionRoutesRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    transactionRoute: CreateTransactionRoutesDto
  ): Promise<TransactionRoutesDto> {
    const transactionRouteEntity: TransactionRoutesEntity =
      TransactionRoutesMapper.toDomain(transactionRoute)
    const transactionRouteCreated =
      await this.transactionRoutesRepository.create(
        organizationId,
        ledgerId,
        transactionRouteEntity
      )

    return TransactionRoutesMapper.toDto(transactionRouteCreated)
  }
}
