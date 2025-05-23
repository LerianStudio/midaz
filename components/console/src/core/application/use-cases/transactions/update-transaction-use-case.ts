import { UpdateTransactionRepository } from '@/core/domain/repositories/transactions/update-transaction-repository'
import { inject, injectable } from 'inversify'
import { TransactionEntity } from '@/core/domain/entities/transaction-entity'
import { TransactionMapper } from '../../mappers/transaction-mapper'
import { UpdateTransactionDto } from '../../dto/update-transaction-dto'
import { TransactionResponseDto } from '../../dto/transaction-dto'
import { LogOperation } from '../../decorators/log-operation'

export interface UpdateTransaction {
  execute: (
    organizationId: string,
    ledgerId: string,
    transactionId: string,
    transaction: Partial<UpdateTransactionDto>
  ) => Promise<TransactionResponseDto>
}

@injectable()
export class UpdateTransactionUseCase implements UpdateTransaction {
  constructor(
    @inject(UpdateTransactionRepository)
    private readonly updateTransactionRepository: UpdateTransactionRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    transactionId: string,
    transaction: Partial<UpdateTransactionDto>
  ): Promise<TransactionResponseDto> {
    const transactionEntity: Partial<TransactionEntity> =
      TransactionMapper.transactionMapperUpdate(
        transaction.description ?? '',
        transaction.metadata ?? {}
      )

    const updatedTransaction: TransactionEntity =
      await this.updateTransactionRepository.update(
        organizationId,
        ledgerId,
        transactionId,
        transactionEntity
      )

    const updatedTransactionResponseDto: TransactionResponseDto =
      TransactionMapper.toResponseDto(updatedTransaction)!

    return updatedTransactionResponseDto
  }
}
