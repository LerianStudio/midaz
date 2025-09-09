// Legacy pagination (deprecated - use cursor pagination)
export type SearchParamDto = {
  page?: number
  limit?: number
}

// New cursor-based pagination (preferred)
export type CursorSearchParamDto = {
  cursor?: string
  limit?: number
  sortOrder?: 'asc' | 'desc'
}
