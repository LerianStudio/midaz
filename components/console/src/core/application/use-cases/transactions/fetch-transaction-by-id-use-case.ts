import 'reflect-metadata'
import { FetchTransactionByIdRepository } from '@/core/domain/repositories/transactions/fetch-transaction-by-id-repository'
import { injectable, inject } from 'inversify'
import { TransactionResponseDto } from '../../dto/transaction-dto'
import { TransactionMapper } from '../../mappers/transaction-mapper'
import { LogOperation } from '../../decorators/log-operation'

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
    @inject(FetchTransactionByIdRepository)
    private readonly fetchTransactionByIdRepository: FetchTransactionByIdRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    transactionId: string
  ): Promise<TransactionResponseDto> {
    const transaction = await this.fetchTransactionByIdRepository.fetchById(
      organizationId,
      ledgerId,
      transactionId
    )

    return TransactionMapper.toResponseDto(transaction)
  }
}
