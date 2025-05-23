import { useState } from 'react'

export type UsePaginationProps = {
  total?: number
}

export function usePagination({ total = 1 }: UsePaginationProps) {
  const [page, _setPage] = useState(1)
  const [limit, _setLimit] = useState(10)

  const totalPages = Math.ceil(total / limit)

  const nextPage = () => {
    if (page + 1 > totalPages) {
      return
    }

    _setPage((page) => page + 1)
  }

  const previousPage = () => {
    if (page - 1 < 1) {
      return
    }

    _setPage((page) => page - 1)
  }

  const setPage = (page: number) => {
    if (page < 1 || page > totalPages) {
      return
    }

    _setPage(page)
  }

  const setLimit = (limit: number) => {
    if (limit < 1) {
      return
    }

    _setLimit(limit)
  }

  return {
    page,
    limit,
    setLimit,
    setPage,
    nextPage,
    previousPage
  }
}

export type UsePaginationReturn = ReturnType<typeof usePagination>
