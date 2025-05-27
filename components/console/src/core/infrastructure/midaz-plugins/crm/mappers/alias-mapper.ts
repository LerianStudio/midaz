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
    dto: AliasPaginatedResponseDto | PaginationEntity<AliasDto>
  ): PaginationEntity<AliasEntity> {
    // Handle both response formats
    if ('data' in dto && 'pagination' in dto) {
      // Expected format with data and pagination
      return {
        items: dto.data.map((alias) => AliasMapper.toEntity(alias)),
        page: dto.pagination.page,
        limit: dto.pagination.limit
      }
    } else if ('items' in dto) {
      // Actual CRM service format (PaginationEntity<AliasDto>)
      return {
        items: (dto.items || []).map((alias: AliasDto) =>
          AliasMapper.toEntity(alias)
        ),
        page: dto.page || 1,
        limit: dto.limit || 10
      }
    } else {
      // Fallback for unexpected format
      return {
        items: [],
        page: 1,
        limit: 10
      }
    }
  }

  static toCreateDto(entity: CreateAliasEntity): CreateAliasDto {
    return {
      name: entity.name,
      type: entity.type,
      ledgerId: entity.ledgerId,
      accountId: entity.accountId,
      metadata: entity.metadata
        ? (entity.metadata as Record<string, string>)
        : undefined,
      bankAccount: entity.bankAccount
    }
  }

  static toUpdateDto(entity: UpdateAliasEntity): UpdateAliasDto {
    return {
      name: entity.name,
      type: entity.type,
      metadata: entity.metadata
        ? (entity.metadata as Record<string, string>)
        : undefined,
      bankAccount: entity.bankAccount
    }
  }
}
