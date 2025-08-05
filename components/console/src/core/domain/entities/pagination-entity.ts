export type PaginationSearchEntity = {
  limit?: number
  page?: number
}

export interface PaginationEntity<T> {
  items: T[]
  limit: number
  page: number
}
