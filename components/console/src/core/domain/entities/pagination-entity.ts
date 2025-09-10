export type PaginationSearchEntity = {
  limit?: number
  page?: number
}

export type SortableSearchEntity = PaginationSearchEntity & {
  sortBy?: string
  sortOrder?: 'asc' | 'desc'
}

export interface PaginationEntity<T> {
  items: T[]
  limit: number
  page: number
}
