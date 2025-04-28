import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { PaginationDto } from '../dto/pagination-dto'

export class PaginationMapper {
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
}
