export type MidazPaginationDto<T> = {
  items: T[]
  limit: number
  page: number
}
