import { TransactionRoutesEntity } from '@/core/domain/entities/transaction-routes-entity'
import {
  MidazTransactionRoutesDto,
  MidazCreateTransactionRoutesDto,
  MidazUpdateTransactionRoutesDto
} from '../dto/midaz-transaction-routes-dto'
import { MidazPaginationDto } from '../dto/midaz-pagination-dto'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { MidazPaginationMapper } from './midaz-pagination-mapper'
import { MidazOperationRoutesMapper } from './midaz-operation-routes-mapper'

export class MidazTransactionRoutesMapper {
  public static toCreateDto(
    transactionRoute: TransactionRoutesEntity
  ): MidazCreateTransactionRoutesDto {
    return {
      title: transactionRoute.title,
      description: transactionRoute.description,
      operationRoutes: transactionRoute.operationRoutes.map((op) => op.id!),
      metadata: transactionRoute.metadata ?? null
    }
  }

  public static toUpdateDto(
    transactionRoute: Partial<TransactionRoutesEntity>
  ): MidazUpdateTransactionRoutesDto {
    return {
      title: transactionRoute.title,
      description: transactionRoute.description,
      operationRoutes: transactionRoute.operationRoutes?.map((op) => op.id!),
      metadata: transactionRoute.metadata ?? null
    }
  }

  public static toEntity(
    dto: MidazTransactionRoutesDto
  ): TransactionRoutesEntity {
    return {
      id: dto.id,
      ledgerId: dto.ledgerId,
      organizationId: dto.organizationId,
      title: dto.title,
      description: dto.description,
      operationRoutes: dto.operationRoutes
        ? dto.operationRoutes.map((op) =>
            MidazOperationRoutesMapper.toEntity(op)
          )
        : [],
      metadata: dto.metadata ?? null,
      createdAt: dto.createdAt ? new Date(dto.createdAt) : undefined,
      updatedAt: dto.updatedAt ? new Date(dto.updatedAt) : undefined,
      deletedAt: dto.deletedAt ? new Date(dto.deletedAt) : undefined
    }
  }

  public static toPaginationEntity(
    result: MidazPaginationDto<MidazTransactionRoutesDto>
  ): PaginationEntity<TransactionRoutesEntity> {
    return MidazPaginationMapper.toResponseDto(
      result,
      MidazTransactionRoutesMapper.toEntity
    )
  }
}
