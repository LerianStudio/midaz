import {
  TransactionRoutesEntity,
  TransactionRoutesSearchEntity
} from '@/core/domain/entities/transaction-routes-entity'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
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
      operationRoutes:
        Array.isArray(entity.operationRoutes) &&
        entity.operationRoutes.length > 0 &&
        typeof entity.operationRoutes[0] === 'object'
          ? (entity.operationRoutes as OperationRoutesEntity[]).map((op) =>
              OperationRoutesMapper.toDto(op)
            )
          : [],
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
      operationRoutes: dto.operationRoutes || [],
      metadata: dto.metadata ?? null
    }
  }

  static toSearchDomain(
    dto: TransactionRoutesSearchParamDto
  ): TransactionRoutesSearchEntity {
    return {
      limit: dto.limit
    }
  }

  static toPaginationResponseDto(
    result: PaginationEntity<TransactionRoutesEntity>
  ): PaginationEntity<TransactionRoutesDto> {
    return PaginationMapper.toResponseDto(result, TransactionRoutesMapper.toDto)
  }
}
