import {
  AccountTypesEntity,
  AccountTypesSearchEntity
} from '@/core/domain/entities/account-types-entity'
import {
  AccountTypesDto,
  AccountTypesSearchParamDto,
  CreateAccountTypesDto,
  UpdateAccountTypesDto
} from '../dto/account-types-dto'
import { CursorPaginationEntity } from '@/core/domain/entities/pagination-entity'
import { PaginationMapper } from './pagination-mapper'

export class AccountTypesMapper {
  public static toDto(accountType: AccountTypesEntity): AccountTypesDto {
    return {
      id: accountType.id!,
      ledgerId: accountType.ledgerId!,
      organizationId: accountType.organizationId!,
      name: accountType.name,
      description: accountType.description,
      keyValue: accountType.keyValue,
      metadata: accountType.metadata ?? null,
      createdAt: accountType.createdAt!,
      updatedAt: accountType.updatedAt!,
      deletedAt: accountType.deletedAt ?? null
    }
  }

  public static toDomain(
    dto: CreateAccountTypesDto | UpdateAccountTypesDto
  ): AccountTypesEntity {
    return {
      name: dto.name!,
      description: dto.description,
      keyValue: (dto as CreateAccountTypesDto).keyValue,
      metadata: dto.metadata ?? null
    }
  }

  static toSearchDomain(
    dto: AccountTypesSearchParamDto
  ): AccountTypesSearchEntity {
    return {
      limit: dto.limit,
      cursor: dto.cursor,
      sortOrder: dto.sortOrder,
      sortBy: dto.sortBy,
      id: dto.id,
      name: dto.name,
      keyValue: dto.keyValue
    }
  }

  static toCursorPaginationResponseDto(
    result: CursorPaginationEntity<AccountTypesEntity>
  ): CursorPaginationEntity<AccountTypesDto> {
    return PaginationMapper.toCursorResponseDto(
      result,
      AccountTypesMapper.toDto
    )
  }
}
