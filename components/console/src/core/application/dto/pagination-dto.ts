// Legacy pagination (deprecated - use cursor pagination)
export interface PaginationDto<T> {
  items: T[]
  limit: number
  page: number
}

// New cursor pagination (preferred)
export interface CursorPaginationDto<T> {
  items: T[]
  limit: number
  nextCursor?: string
  prevCursor?: string
}
