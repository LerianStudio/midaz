import { 
  TransactionEntity, 
  TransactionSearchEntity 
} from '@/core/domain/entities/transaction-entity'
import { 
  CreateTransactionDto, 
  TransactionDto, 
  TransactionSearchDto 
} from '../dto/transaction-dto'
import { CursorPaginationEntity } from '@/core/domain/entities/pagination-entity'
import { PaginationMapper } from './pagination-mapper'
import { CursorPaginationDto } from '../dto/pagination-dto'

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
    return {
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
  }

  static toSearchDomain(dto: TransactionSearchDto): TransactionSearchEntity {
    return {
      limit: dto.limit,
      cursor: dto.cursor,
      sortOrder: dto.sortOrder,
      sortBy: dto.sortBy,
      id: dto.id
    }
  }

  static toCursorPaginationResponseDto(
    result: CursorPaginationEntity<TransactionEntity>
  ): CursorPaginationEntity<TransactionDto> {
    return PaginationMapper.toCursorResponseDto(
      result,
      TransactionMapper.toResponseDto
    )
  }
}
