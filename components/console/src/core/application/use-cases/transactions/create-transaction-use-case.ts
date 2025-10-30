import { TransactionRepository } from '@/core/domain/repositories/transaction-repository'
import { inject, injectable } from 'inversify'
import { v7 as uuidv7 } from 'uuid'
import { TransactionMapper } from '../../mappers/transaction-mapper'
import type {
  CreateTransactionDto,
  TransactionDto
} from '../../dto/transaction-dto'
import { TransactionEntity } from '@/core/domain/entities/transaction-entity'
import { LogOperation } from '@/core/infrastructure/logger/decorators/log-operation'

export interface CreateTransaction {
  execute: (
    organizationId: string,
    ledgerId: string,
    transaction: CreateTransactionDto
  ) => Promise<TransactionDto>
}

@injectable()
export class CreateTransactionUseCase implements CreateTransaction {
  constructor(
    @inject(TransactionRepository)
    private readonly transactionRepository: TransactionRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    transaction: CreateTransactionDto
  ): Promise<TransactionDto> {
    const transactionEntity: TransactionEntity =
      TransactionMapper.toDomain(transaction)

    const idempotencyKey = uuidv7()

    const transactionCreated = await this.transactionRepository.create(
      organizationId,
      ledgerId,
      transactionEntity,
      idempotencyKey
    )

    const transactionResponseDto: TransactionDto =
      TransactionMapper.toResponseDto(transactionCreated)

    return transactionResponseDto
  }
}
