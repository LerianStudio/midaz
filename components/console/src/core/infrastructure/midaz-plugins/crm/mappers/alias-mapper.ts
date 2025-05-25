import {
  AliasEntity,
  CreateAliasEntity,
  UpdateAliasEntity
} from '@/core/domain/entities/alias-entity'
import {
  AliasDto,
  AliasPaginatedResponseDto,
  CreateAliasDto,
  UpdateAliasDto
} from '../dto/alias-dto'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'

export class AliasMapper {
  static toEntity(dto: AliasDto): AliasEntity {
    return {
      id: dto.id,
      name: dto.name,
      type: dto.type,
      ledgerId: dto.ledgerId,
      accountId: dto.accountId,
      metadata: dto.metadata,
      bankAccount: dto.bankAccount,
      createdAt: dto.createdAt,
      updatedAt: dto.updatedAt,
      deletedAt: dto.deletedAt
    }
  }

  static toPaginatedEntity(
    dto: AliasPaginatedResponseDto
  ): PaginationEntity<AliasEntity> {
    return {
      items: dto.data.map((alias) => AliasMapper.toEntity(alias)),
      page: dto.pagination.page,
      limit: dto.pagination.limit,
      total: dto.pagination.total,
      totalPages: dto.pagination.totalPages
    }
  }

  static toCreateDto(entity: CreateAliasEntity): CreateAliasDto {
    return {
      name: entity.name,
      type: entity.type,
      ledgerId: entity.ledgerId,
      accountId: entity.accountId,
      metadata: entity.metadata,
      bankAccount: entity.bankAccount
    }
  }

  static toUpdateDto(entity: UpdateAliasEntity): UpdateAliasDto {
    return {
      name: entity.name,
      type: entity.type,
      metadata: entity.metadata,
      bankAccount: entity.bankAccount
    }
  }
}
