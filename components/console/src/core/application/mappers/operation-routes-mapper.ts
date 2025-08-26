import { OperationRoutesEntity, OperationRoutesSearchEntity } from '@/core/domain/entities/operation-routes-entity'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import {
  CreateOperationRoutesDto,
  UpdateOperationRoutesDto,
  OperationRoutesDto,
  OperationRoutesSearchParamDto,
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
      account: entity.account,
      metadata: entity.metadata ?? null,
      createdAt: entity.createdAt!,
      updatedAt: entity.updatedAt!,
      deletedAt: entity.deletedAt
    }
  }

  public static toDomain(dto: CreateOperationRoutesDto | UpdateOperationRoutesDto): OperationRoutesEntity {
    return {  
      title: dto.title!,
      description: dto.description!,
      account: dto.account!,
      metadata: dto.metadata! ?? null,
      operationType: dto.operationType!,
    }
  }

  static toSearchDomain(dto: OperationRoutesSearchParamDto): OperationRoutesSearchEntity {
    return {
      limit: dto.limit,
      start_date: dto.start_date,
      end_date: dto.end_date,
      sort_order: dto.sort_order,
      cursor: dto.cursor,
      metadata: dto.metadata
    }
  }

  static toPaginationResponseDto(
    result: PaginationEntity<OperationRoutesEntity>
  ): PaginationEntity<OperationRoutesDto> {
    return PaginationMapper.toResponseDto(result, OperationRoutesMapper.toDto)
  }
}
