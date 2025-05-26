export type IdentityPaginationDto<T> = {
  items: T[]
  limit: number
  page: number
}
