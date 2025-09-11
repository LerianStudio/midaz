// Legacy pagination (deprecated - use cursor pagination)
export type PaginationSearchEntity = {
  limit?: number
  page?: number
}

// New cursor-based pagination (preferred)
export type CursorSearchEntity = {
  limit?: number
  cursor?: string
  sortOrder?: 'desc' | 'asc'
}

export type SortableSearchEntity = PaginationSearchEntity & {
  sortBy?: string
  sortOrder?: 'desc' | 'asc'
}

// New cursor-based sortable search
export type CursorSortableSearchEntity = CursorSearchEntity & {
  sortBy?: string
}

// Legacy pagination entity (deprecated - use cursor pagination)
export interface PaginationEntity<T> {
  items: T[]
  limit: number
  page: number
}

// New cursor pagination entity (preferred)
export interface CursorPaginationEntity<T> {
  items: T[]
  limit: number
  nextCursor?: string
  prevCursor?: string
}
