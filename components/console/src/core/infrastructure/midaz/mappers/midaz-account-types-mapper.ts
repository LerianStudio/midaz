import { AccountTypesEntity } from '@/core/domain/entities/account-types-entity'
import {
  MidazAccountTypesDto,
  MidazCreateAccountTypesDto,
  MidazUpdateAccountTypesDto
} from '../dto/midaz-account-types-dto'
import {
  MidazCursorPaginationDto,
  MidazPaginationDto
} from '../dto/midaz-pagination-dto'
import {
  CursorPaginationEntity,
  PaginationEntity
} from '@/core/domain/entities/pagination-entity'
import { MidazPaginationMapper } from './midaz-pagination-mapper'

export class MidazAccountTypesMapper {
  public static toCreateDto(
    accountType: AccountTypesEntity
  ): MidazCreateAccountTypesDto {
    return {
      name: accountType.name,
      description: accountType.description,
      keyValue: accountType.keyValue,
      metadata: accountType.metadata
    }
  }

  public static toUpdateDto(
    accountType: Partial<AccountTypesEntity>
  ): MidazUpdateAccountTypesDto {
    return {
      name: accountType.name,
      description: accountType.description,
      metadata: accountType.metadata
    }
  }

  public static toEntity(dto: MidazAccountTypesDto): AccountTypesEntity {
    return {
      id: dto.id,
      ledgerId: dto.ledgerId,
      organizationId: dto.organizationId,
      name: dto.name,
      description: dto.description,
      keyValue: dto.keyValue,
      metadata: dto.metadata ?? null,
      createdAt: dto.createdAt,
      updatedAt: dto.updatedAt,
      deletedAt: dto.deletedAt
    }
  }

  public static toCursorPaginationEntity(
    result: MidazCursorPaginationDto<MidazAccountTypesDto>
  ): CursorPaginationEntity<AccountTypesEntity> {
    return MidazPaginationMapper.toCursorResponseDto(
      result,
      MidazAccountTypesMapper.toEntity
    )
  }

  public static toPaginationEntity(
    result: MidazPaginationDto<MidazAccountTypesDto>
  ): PaginationEntity<AccountTypesEntity> {
    return MidazPaginationMapper.toResponseDto(
      result,
      MidazAccountTypesMapper.toEntity
    )
  }
}
