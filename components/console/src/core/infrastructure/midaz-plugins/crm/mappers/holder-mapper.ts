import {
  CreateHolderEntity,
  HolderEntity,
  UpdateHolderEntity
} from '@/core/domain/entities/holder-entity'
import {
  CreateHolderDto,
  HolderDto,
  HolderPaginatedResponseDto,
  UpdateHolderDto
} from '../dto/holder-dto'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'

export class HolderMapper {
  static toEntity(dto: HolderDto): HolderEntity {
    return {
      id: dto.id,
      name: dto.name,
      status: dto.status,
      type: dto.type,
      document: dto.document,
      address: dto.address,
      tradingName: dto.tradingName,
      legalName: dto.legalName,
      website: dto.website,
      establishedOn: dto.establishedOn,
      monthlyIncomeTotal: dto.monthlyIncomeTotal,
      contacts: dto.contacts,
      metadata: dto.metadata,
      createdAt: dto.createdAt,
      updatedAt: dto.updatedAt,
      deletedAt: dto.deletedAt
    }
  }

  static toPaginatedEntity(
    dto: HolderPaginatedResponseDto | any
  ): PaginationEntity<HolderEntity> {
    // Handle both response formats
    if (dto.data && dto.pagination) {
      // Expected format with data and pagination
      return {
        items: dto.data.map((holder: HolderDto) =>
          HolderMapper.toEntity(holder)
        ),
        page: dto.pagination.page,
        limit: dto.pagination.limit,
        total: dto.pagination.total,
        totalPages: dto.pagination.totalPages
      }
    } else if (dto.items !== undefined) {
      // Actual CRM service format
      return {
        items: (dto.items || []).map((holder: HolderDto) =>
          HolderMapper.toEntity(holder)
        ),
        page: dto.page || 1,
        limit: dto.limit || 10,
        total: dto.total || dto.items?.length || 0,
        totalPages:
          dto.totalPages ||
          Math.ceil((dto.total || dto.items?.length || 0) / (dto.limit || 10))
      }
    } else {
      // Fallback for unexpected format
      return {
        items: [],
        page: 1,
        limit: 10,
        total: 0,
        totalPages: 0
      }
    }
  }

  static toCreateDto(entity: CreateHolderEntity): CreateHolderDto {
    return {
      name: entity.name,
      type: entity.type,
      document: entity.document,
      status: entity.status,
      address: entity.address,
      tradingName: entity.tradingName,
      legalName: entity.legalName,
      website: entity.website,
      establishedOn: entity.establishedOn,
      monthlyIncomeTotal: entity.monthlyIncomeTotal,
      contacts: entity.contacts,
      metadata: entity.metadata
    }
  }

  static toUpdateDto(entity: UpdateHolderEntity): UpdateHolderDto {
    return {
      name: entity.name,
      status: entity.status,
      address: entity.address,
      tradingName: entity.tradingName,
      legalName: entity.legalName,
      website: entity.website,
      establishedOn: entity.establishedOn,
      monthlyIncomeTotal: entity.monthlyIncomeTotal,
      contacts: entity.contacts,
      metadata: entity.metadata
    }
  }
}
