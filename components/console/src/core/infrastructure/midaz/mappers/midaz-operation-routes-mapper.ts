import { OperationRoutesEntity } from '@/core/domain/entities/operation-routes-entity'
import {
  MidazOperationRoutesDto,
  MidazCreateOperationRoutesDto,
  MidazUpdateOperationRoutesDto
} from '../dto/midaz-operation-routes-dto'
import {
  MidazCursorPaginationDto,
  MidazPaginationDto
} from '../dto/midaz-pagination-dto'
import {
  CursorPaginationEntity,
  PaginationEntity
} from '@/core/domain/entities/pagination-entity'
import { MidazPaginationMapper } from './midaz-pagination-mapper'

export class MidazOperationRoutesMapper {
  public static toCreateDto(
    operationRoute: OperationRoutesEntity
  ): MidazCreateOperationRoutesDto {
    return {
      title: operationRoute.title,
      description: operationRoute.description,
      operationType: operationRoute.operationType,
      account: operationRoute.account,
      metadata: operationRoute.metadata ?? null
    }
  }

  public static toUpdateDto(
    operationRoute: Partial<OperationRoutesEntity>
  ): MidazUpdateOperationRoutesDto {
    return {
      title: operationRoute.title,
      description: operationRoute.description,
      operationType: operationRoute.operationType,
      account: operationRoute.account,
      metadata: operationRoute.metadata ?? null
    }
  }

  public static toEntity(dto: MidazOperationRoutesDto): OperationRoutesEntity {
    return {
      id: dto.id,
      ledgerId: dto.ledgerId,
      organizationId: dto.organizationId,
      title: dto.title,
      description: dto.description,
      operationType: dto.operationType,
      account: dto.account,
      metadata: dto.metadata ?? null,
      createdAt: dto.createdAt ? new Date(dto.createdAt) : undefined,
      updatedAt: dto.updatedAt ? new Date(dto.updatedAt) : undefined,
      deletedAt: dto.deletedAt ? new Date(dto.deletedAt) : undefined
    }
  }

  public static toCursorPaginationEntity(
    result: MidazCursorPaginationDto<MidazOperationRoutesDto>
  ): CursorPaginationEntity<OperationRoutesEntity> {
    return MidazPaginationMapper.toCursorResponseDto(
      result,
      MidazOperationRoutesMapper.toEntity
    )
  }

  public static toPaginationEntity(
    result: MidazPaginationDto<MidazOperationRoutesDto>
  ): PaginationEntity<OperationRoutesEntity> {
    return MidazPaginationMapper.toResponseDto(
      result,
      MidazOperationRoutesMapper.toEntity
    )
  }
}
