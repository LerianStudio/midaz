import {
  TransactionRoutesEntity,
  TransactionRoutesSearchEntity
} from '@/core/domain/entities/transaction-routes-entity'
import { CursorPaginationEntity } from '@/core/domain/entities/pagination-entity'
import {
  CreateTransactionRoutesDto,
  UpdateTransactionRoutesDto,
  TransactionRoutesDto,
  TransactionRoutesSearchParamDto
} from '../dto/transaction-routes-dto'
import { PaginationMapper } from './pagination-mapper'
import { OperationRoutesMapper } from './operation-routes-mapper'
import { OperationRoutesEntity } from '@/core/domain/entities/operation-routes-entity'

export class TransactionRoutesMapper {
  public static toDto(entity: TransactionRoutesEntity): TransactionRoutesDto {
    return {
      id: entity.id!,
      organizationId: entity.organizationId!,
      ledgerId: entity.ledgerId!,
      title: entity.title,
      description: entity.description,
      operationRoutes: entity.operationRoutes.map(OperationRoutesMapper.toDto),
      metadata: entity.metadata ?? null,
      createdAt: entity.createdAt!.toISOString(),
      updatedAt: entity.updatedAt!.toISOString(),
      deletedAt: entity.deletedAt?.toISOString()
    }
  }

  public static toDomain(
    dto: CreateTransactionRoutesDto | UpdateTransactionRoutesDto
  ): TransactionRoutesEntity {
    return {
      title: dto.title!,
      description: dto.description,
      operationRoutes: (dto.operationRoutes || []).map(
        (id) =>
          ({
            id: id,
            title: '',
            description: ''
          }) as OperationRoutesEntity
      ),
      metadata: dto.metadata ?? null
    }
  }

  static toSearchDomain(
    dto: TransactionRoutesSearchParamDto
  ): TransactionRoutesSearchEntity {
    return {
      limit: dto.limit,
      cursor: dto.cursor,
      sortOrder: dto.sortOrder,
      sortBy: dto.sortBy,
      id: dto.id
    }
  }

  static toCursorPaginationResponseDto(
    result: CursorPaginationEntity<TransactionRoutesEntity>
  ): CursorPaginationEntity<TransactionRoutesDto> {
    return PaginationMapper.toCursorResponseDto(
      result,
      TransactionRoutesMapper.toDto
    )
  }
}
