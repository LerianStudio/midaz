import { TransactionEntity } from '@/core/domain/entities/transaction-entity'
import { CreateTransactionDto, TransactionDto } from '../dto/transaction-dto'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { PaginationMapper } from './pagination-mapper'
import { PaginationDto } from '../dto/pagination-dto'

export class TransactionMapper {
  static toDomain(transaction: CreateTransactionDto): TransactionEntity {
    return {
      description: transaction.description,
      chartOfAccountsGroupName: transaction.chartOfAccountsGroupName,
      amount: transaction.amount,
      asset: transaction.asset,
      source: transaction.source,
      destination: transaction.destination,
      metadata: transaction.metadata
    }
  }

  static toResponseDto(transaction: TransactionEntity): TransactionDto {
    console.log('=== TRANSACTION MAPPER toResponseDto ===')
    console.log('Input transaction:', JSON.stringify(transaction, null, 2))

    // Check if this is a fee API response (has segmentId and transaction properties)
    if (
      transaction &&
      typeof transaction === 'object' &&
      'segmentId' in transaction &&
      'transaction' in transaction
    ) {
      console.log('Detected fee API response, returning as-is')
      return transaction as any
    }

    const result = {
      id: transaction.id!,
      ledgerId: transaction.ledgerId!,
      organizationId: transaction.organizationId!,
      description: transaction.description,
      chartOfAccountsGroupName: transaction.chartOfAccountsGroupName,
      status: transaction.status!,
      amount: transaction.amount,
      asset: transaction.asset,
      source: transaction.source,
      destination: transaction.destination,
      metadata: transaction.metadata,
      createdAt: transaction.createdAt!,
      updatedAt: transaction.updatedAt,
      deletedAt: transaction.deletedAt
    }

    console.log('Returning result:', JSON.stringify(result, null, 2))
    return result
  }

  static toPaginatedResponseDto(
    paginationEntity: PaginationEntity<TransactionEntity>
  ): PaginationDto<TransactionDto> {
    return PaginationMapper.toResponseDto(
      paginationEntity,
      TransactionMapper.toResponseDto as (
        transaction: TransactionEntity
      ) => TransactionDto
    )
  }
}
