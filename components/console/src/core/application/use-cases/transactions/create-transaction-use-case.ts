import { CreateTransactionRepository } from '@/core/domain/repositories/transactions/create-transaction-repository'
import { inject, injectable } from 'inversify'
import { TransactionMapper } from '../../mappers/transaction-mapper'
import type {
  CreateTransactionDto,
  TransactionResponseDto
} from '../../dto/transaction-dto'
import {
  TransactionCreateEntity,
  TransactionEntity
} from '@/core/domain/entities/transaction-entity'
import { LogOperation } from '../../decorators/log-operation'

export interface CreateTransaction {
  execute: (
    organizationId: string,
    ledgerId: string,
    transaction: CreateTransactionDto
  ) => Promise<TransactionResponseDto>
}

@injectable()
export class CreateTransactionUseCase implements CreateTransaction {
  constructor(
    @inject(CreateTransactionRepository)
    private readonly createTransactionRepository: CreateTransactionRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    transaction: CreateTransactionDto
  ): Promise<TransactionResponseDto> {
    const transactionEntity: TransactionCreateEntity =
      TransactionMapper.toDomain(transaction)

    const transactionCreated = await this.createTransactionRepository.create(
      organizationId,
      ledgerId,
      transactionEntity
    )

    const transactionResponseDto: TransactionResponseDto =
      TransactionMapper.toResponseDto(transactionCreated)

    return transactionResponseDto
  }
}
