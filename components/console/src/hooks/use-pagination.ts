import { useState, useCallback } from 'react'

export type UsePaginationProps = {
  total?: number
  initialPage?: number
  initialLimit?: number
}

export function usePagination({ 
  total = 1, 
  initialPage = 1, 
  initialLimit = 10 
}: UsePaginationProps) {
  const [page, _setPage] = useState(initialPage)
  const [limit, _setLimit] = useState(initialLimit)

  const totalPages = Math.ceil(total / limit)

  const nextPage = useCallback(() => {
    if (page + 1 > totalPages) {
      return
    }

    _setPage((page) => page + 1)
  }, [page, totalPages])

  const previousPage = useCallback(() => {
    if (page - 1 < 1) {
      return
    }

    _setPage((page) => page - 1)
  }, [page])

  const setPage = useCallback((newPage: number) => {
    if (newPage < 1 || (totalPages > 0 && newPage > totalPages)) {
      return
    }

    _setPage(newPage)
  }, [totalPages])

  const setLimit = useCallback((newLimit: number) => {
    if (newLimit < 1) {
      return
    }

    // Reset to page 1 when changing limit to avoid invalid page states
    _setLimit(newLimit)
    _setPage(1)
  }, [])

  // Reset to page 1 if current page becomes invalid due to total change
  const safeCurrentPage = totalPages > 0 && page > totalPages ? 1 : page

  return {
    page: safeCurrentPage,
    limit,
    totalPages,
    setLimit,
    setPage,
    nextPage,
    previousPage
  }
}

export type UsePaginationReturn = ReturnType<typeof usePagination>
