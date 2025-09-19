import { TransactionRoutesRepository } from '@/core/domain/repositories/transaction-routes-repository'
import {
  TransactionRoutesDto,
  UpdateTransactionRoutesDto
} from '@/core/application/dto/transaction-routes-dto'
import { TransactionRoutesMapper } from '@/core/application/mappers/transaction-routes-mapper'
import { TransactionRoutesEntity } from '@/core/domain/entities/transaction-routes-entity'
import { inject, injectable } from 'inversify'
import { LogOperation } from '@/core/infrastructure/logger/decorators/log-operation'

export interface UpdateTransactionRoutes {
  execute: (
    organizationId: string,
    ledgerId: string,
    transactionRouteId: string,
    transactionRoute: Partial<UpdateTransactionRoutesDto>
  ) => Promise<TransactionRoutesDto>
}

@injectable()
export class UpdateTransactionRoutesUseCase implements UpdateTransactionRoutes {
  constructor(
    @inject(TransactionRoutesRepository)
    private readonly transactionRoutesRepository: TransactionRoutesRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    transactionRouteId: string,
    transactionRoute: Partial<UpdateTransactionRoutesDto>
  ): Promise<TransactionRoutesDto> {
    const transactionRouteEntity: Partial<TransactionRoutesEntity> =
      TransactionRoutesMapper.toDomain(transactionRoute)

    const updatedTransactionRoute: TransactionRoutesEntity =
      await this.transactionRoutesRepository.update(
        organizationId,
        ledgerId,
        transactionRouteId,
        transactionRouteEntity
      )

    return TransactionRoutesMapper.toDto(updatedTransactionRoute)
  }
}
