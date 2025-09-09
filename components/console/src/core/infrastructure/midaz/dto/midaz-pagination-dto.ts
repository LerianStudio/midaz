// Legacy pagination (deprecated - use cursor pagination)
export type MidazPaginationDto<T> = {
  items: T[]
  limit: number
  page: number
}

// New cursor pagination (preferred) - matches Midaz API response
export type MidazCursorPaginationDto<T> = {
  items: T[]
  limit: number
  next_cursor?: string
  prev_cursor?: string
}
