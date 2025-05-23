export interface PaginationDto<T> {
  items: T[]
  limit: number
  page: number
}
