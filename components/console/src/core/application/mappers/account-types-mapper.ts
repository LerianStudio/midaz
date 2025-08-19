import { AccountTypesEntity } from '@/core/domain/entities/account-types-entity'
import {
  AccountTypesDto,
  CreateAccountTypesDto,
  UpdateAccountTypesDto
} from '../dto/account-types-dto'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
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

  static toPaginationResponseDto(
    result: PaginationEntity<AccountTypesEntity>
  ): PaginationEntity<AccountTypesDto> {
    return PaginationMapper.toResponseDto(result, AccountTypesMapper.toDto)
  }
}
