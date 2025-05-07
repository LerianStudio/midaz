export interface MidazPaginationDto<T> {
  items: T[]
  limit: number
  page: number
}
