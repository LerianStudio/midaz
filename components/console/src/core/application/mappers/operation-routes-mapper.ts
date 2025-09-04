import {
  OperationRoutesEntity,
  OperationRoutesSearchEntity
} from '@/core/domain/entities/operation-routes-entity'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import {
  CreateOperationRoutesDto,
  UpdateOperationRoutesDto,
  OperationRoutesDto,
  OperationRoutesSearchParamDto
} from '../dto/operation-routes-dto'
import { PaginationMapper } from './pagination-mapper'

export class OperationRoutesMapper {
  public static toDto(entity: OperationRoutesEntity): OperationRoutesDto {
    return {
      id: entity.id!,
      organizationId: entity.organizationId!,
      ledgerId: entity.ledgerId!,
      title: entity.title,
      description: entity.description,
      operationType: entity.operationType!,
      account: entity.account,
      metadata: entity.metadata ?? null,
      createdAt: entity.createdAt!.toISOString(),
      updatedAt: entity.updatedAt!.toISOString(),
      deletedAt: entity.deletedAt?.toISOString()
    }
  }

  public static toDomain(
    dto: CreateOperationRoutesDto | UpdateOperationRoutesDto
  ): OperationRoutesEntity {
    return {
      title: dto.title!,
      description: dto.description!,
      account: dto.account!,
      metadata: dto.metadata ?? null,
      operationType: dto.operationType as 'source' | 'destination'
    }
  }

  static toSearchDomain(
    dto: OperationRoutesSearchParamDto
  ): OperationRoutesSearchEntity {
    return {
      limit: dto.limit
    }
  }

  static toPaginationResponseDto(
    result: PaginationEntity<OperationRoutesEntity>
  ): PaginationEntity<OperationRoutesDto> {
    return PaginationMapper.toResponseDto(result, OperationRoutesMapper.toDto)
  }
}
