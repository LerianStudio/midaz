import { TransactionRepository } from '@/core/domain/repositories/transaction-repository'
import { inject, injectable } from 'inversify'
import { TransactionEntity } from '@/core/domain/entities/transaction-entity'
import { TransactionMapper } from '../../mappers/transaction-mapper'
import {
  CreateTransactionDto,
  UpdateTransactionDto
} from '../../dto/transaction-dto'
import { TransactionDto } from '../../dto/transaction-dto'
import { LogOperation } from '@/core/infrastructure/logger/decorators/log-operation'

export interface UpdateTransaction {
  execute: (
    organizationId: string,
    ledgerId: string,
    transactionId: string,
    transaction: Partial<UpdateTransactionDto>
  ) => Promise<TransactionDto>
}

@injectable()
export class UpdateTransactionUseCase implements UpdateTransaction {
  constructor(
    @inject(TransactionRepository)
    private readonly transactionRepository: TransactionRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    transactionId: string,
    transaction: Partial<UpdateTransactionDto>
  ): Promise<TransactionDto> {
    const transactionEntity = TransactionMapper.toDomain({
      description: transaction.description ?? '',
      metadata: transaction.metadata ?? {}
    } as CreateTransactionDto)

    const updatedTransaction: TransactionEntity =
      await this.transactionRepository.update(
        organizationId,
        ledgerId,
        transactionId,
        transactionEntity
      )

    const updatedTransactionResponseDto =
      TransactionMapper.toResponseDto(updatedTransaction)!

    return updatedTransactionResponseDto
  }
}
