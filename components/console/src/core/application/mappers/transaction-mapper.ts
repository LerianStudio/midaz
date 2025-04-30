import { TransactionEntity } from '@/core/domain/entities/transaction-entity'
import {
  CreateTransactionDto,
  TransactionResponseDto
} from '../dto/transaction-dto'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { PaginationMapper } from './pagination-mapper'
import { PaginationDto } from '../dto/pagination-dto'

export class TransactionMapper {
  static toDomain(transaction: CreateTransactionDto): TransactionEntity {
    return {
      description: transaction.description,
      chartOfAccountsGroupName: transaction.chartOfAccountsGroupName,
      value: transaction.value,
      asset: transaction.asset,
      source: transaction.source,
      destination: transaction.destination,
      metadata: transaction.metadata
    }
  }

  static toResponseDto(transaction: TransactionEntity): TransactionResponseDto {
    return {
      id: transaction.id!,
      ledgerId: transaction.ledgerId!,
      organizationId: transaction.organizationId!,
      description: transaction.description,
      chartOfAccountsGroupName: transaction.chartOfAccountsGroupName,
      status: transaction.status!,
      value: transaction.value,
      asset: transaction.asset,
      source: transaction.source,
      destination: transaction.destination,
      metadata: transaction.metadata,
      createdAt: transaction.createdAt!,
      updatedAt: transaction.updatedAt,
      deletedAt: transaction.deletedAt
    }
  }

  static toPaginatedResponseDto(
    paginationEntity: PaginationEntity<TransactionEntity>
  ): PaginationDto<TransactionResponseDto> {
    return PaginationMapper.toResponseDto(
      paginationEntity,
      TransactionMapper.toResponseDto as (
        transaction: TransactionEntity
      ) => TransactionResponseDto
    )
  }
}
