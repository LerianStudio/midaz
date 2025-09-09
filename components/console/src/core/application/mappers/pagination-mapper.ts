import {
  PaginationEntity,
  CursorPaginationEntity
} from '@/core/domain/entities/pagination-entity'
import { PaginationDto, CursorPaginationDto } from '../dto/pagination-dto'

export class PaginationMapper {
  // Legacy pagination mapper (deprecated - use cursor pagination)
  static toResponseDto<T, R = T>(
    paginationEntity: PaginationEntity<T>,
    mapper = (item: T) => item as unknown as R
  ): PaginationDto<R> {
    const items =
      paginationEntity.items && paginationEntity.items !== null
        ? paginationEntity.items.map(mapper)
        : []

    return {
      items: items,
      limit: paginationEntity.limit,
      page: paginationEntity.page
    }
  }

  // New cursor pagination mapper (preferred)
  static toCursorResponseDto<T, R = T>(
    paginationEntity: CursorPaginationEntity<T>,
    mapper = (item: T) => item as unknown as R
  ): CursorPaginationDto<R> {
    const items =
      paginationEntity.items && paginationEntity.items !== null
        ? paginationEntity.items.map(mapper)
        : []

    return {
      items: items,
      limit: paginationEntity.limit,
      nextCursor: paginationEntity.nextCursor,
      prevCursor: paginationEntity.prevCursor
    }
  }
}
