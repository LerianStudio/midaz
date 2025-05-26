export interface PaginationEntity<T> {
  items: T[]
  limit: number
  page: number
}
