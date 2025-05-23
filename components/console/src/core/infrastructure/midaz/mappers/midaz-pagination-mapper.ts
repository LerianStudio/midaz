import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { MidazPaginationDto } from '../dto/midaz-pagination-dto'

export class MidazPaginationMapper {
  static toResponseDto<T, R = T>(
    paginationEntity: PaginationEntity<T>,
    mapper = (item: T) => item as unknown as R
  ): MidazPaginationDto<R> {
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
