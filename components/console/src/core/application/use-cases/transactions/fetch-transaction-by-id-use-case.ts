import 'reflect-metadata'
import { TransactionRepository } from '@/core/domain/repositories/transaction-repository'
import { injectable, inject } from 'inversify'
import { TransactionResponseDto } from '../../dto/transaction-dto'
import { TransactionMapper } from '../../mappers/transaction-mapper'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface FetchTransactionById {
  execute: (
    organizationId: string,
    ledgerId: string,
    transactionId: string
  ) => Promise<TransactionResponseDto>
}

@injectable()
export class FetchTransactionByIdUseCase implements FetchTransactionById {
  constructor(
    @inject(TransactionRepository)
    private readonly transactionRepository: TransactionRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    transactionId: string
  ): Promise<TransactionResponseDto> {
    const transaction = await this.transactionRepository.fetchById(
      organizationId,
      ledgerId,
      transactionId
    )

    return TransactionMapper.toResponseDto(transaction)
  }
}
