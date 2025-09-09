import {
  PaginationEntity,
  CursorPaginationEntity
} from '@/core/domain/entities/pagination-entity'
import {
  MidazPaginationDto,
  MidazCursorPaginationDto
} from '../dto/midaz-pagination-dto'

export class MidazPaginationMapper {
  // Legacy pagination mapping (deprecated - use cursor pagination)
  // Maps FROM Midaz API response TO domain entity
  static toResponseDto<T, R = T>(
    midazPaginationDto: MidazPaginationDto<T>,
    mapper = (item: T) => item as unknown as R
  ): PaginationEntity<R> {
    const items =
      midazPaginationDto.items && midazPaginationDto.items !== null
        ? midazPaginationDto.items.map(mapper)
        : []

    return {
      items: items,
      limit: midazPaginationDto.limit,
      page: midazPaginationDto.page
    }
  }

  // New cursor pagination mapping (preferred)
  // Maps FROM Midaz API response TO domain entity
  static toCursorResponseDto<T, R = T>(
    midazCursorPaginationDto: MidazCursorPaginationDto<T>,
    mapper = (item: T) => item as unknown as R
  ): CursorPaginationEntity<R> {
    const items =
      midazCursorPaginationDto.items && midazCursorPaginationDto.items !== null
        ? midazCursorPaginationDto.items.map(mapper)
        : []

    return {
      items: items,
      limit: midazCursorPaginationDto.limit,
      nextCursor: midazCursorPaginationDto.next_cursor,
      prevCursor: midazCursorPaginationDto.prev_cursor
    }
  }
}
