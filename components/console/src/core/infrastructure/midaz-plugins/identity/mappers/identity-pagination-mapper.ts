import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { IdentityPaginationDto } from '../dto/identity-pagination-dto'

export class IdentityPaginationMapper {
  static toResponseDto<T, R = T>(
    paginationEntity: PaginationEntity<T>,
    mapper = (item: T) => item as unknown as R
  ): IdentityPaginationDto<R> {
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
