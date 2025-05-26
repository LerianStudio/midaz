import 'reflect-metadata'
import { TransactionRepository } from '@/core/domain/repositories/transaction-repository'
import { injectable, inject } from 'inversify'
import { TransactionDto } from '../../dto/transaction-dto'
import { TransactionMapper } from '../../mappers/transaction-mapper'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'
import { MIDAZ_SYMBOLS } from '@/core/infrastructure/container-registry/midaz/midaz-module'

export interface FetchTransactionById {
  execute: (
    organizationId: string,
    ledgerId: string,
    transactionId: string
  ) => Promise<TransactionDto>
}

@injectable()
export class FetchTransactionByIdUseCase implements FetchTransactionById {
  constructor(
    @inject(MIDAZ_SYMBOLS.TransactionRepository)
    private readonly transactionRepository: TransactionRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    transactionId: string
  ): Promise<TransactionDto> {
    const transaction = await this.transactionRepository.fetchById(
      organizationId,
      ledgerId,
      transactionId
    )

    return TransactionMapper.toResponseDto(transaction)
  }
}
