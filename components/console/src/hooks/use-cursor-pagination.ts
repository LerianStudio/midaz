import { useState, useCallback } from 'react'

export type UseCursorPaginationProps = {
  limit?: number
  initialCursor?: string
  sortOrder?: 'asc' | 'desc'
}

export function useCursorPagination({
  limit: initialLimit = 10,
  initialCursor,
  sortOrder: initialSortOrder = 'asc'
}: UseCursorPaginationProps = {}) {
  const [cursor, setCursor] = useState<string | undefined>(initialCursor)
  const [limit, setLimit] = useState(initialLimit)
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>(initialSortOrder)
  const [hasNext, setHasNext] = useState(false)
  const [hasPrev, setHasPrev] = useState(false)
  const [nextCursor, setNextCursor] = useState<string | undefined>()
  const [prevCursor, setPrevCursor] = useState<string | undefined>()

  const updatePaginationState = useCallback(
    (paginationInfo: { next_cursor?: string; prev_cursor?: string }) => {
      setNextCursor(paginationInfo.next_cursor)
      setPrevCursor(paginationInfo.prev_cursor)
      setHasNext(!!paginationInfo.next_cursor)
      setHasPrev(!!paginationInfo.prev_cursor)
    },
    []
  )

  const nextPage = useCallback(() => {
    if (nextCursor) {
      setCursor(nextCursor)
    }
  }, [nextCursor])

  const previousPage = useCallback(() => {
    if (prevCursor) {
      setCursor(prevCursor)
    }
  }, [prevCursor])

  const goToFirstPage = useCallback(() => {
    setCursor(undefined)
  }, [])

  const updateLimit = useCallback((newLimit: number) => {
    if (newLimit > 0) {
      setLimit(newLimit)
      setCursor(undefined) // Reset cursor when limit changes
    }
  }, [])

  const updateSortOrder = useCallback((newSortOrder: 'asc' | 'desc') => {
    setSortOrder(newSortOrder)
    setCursor(undefined) // Reset cursor when sort order changes
  }, [])

  const reset = useCallback(() => {
    setCursor(undefined)
    setHasNext(false)
    setHasPrev(false)
    setNextCursor(undefined)
    setPrevCursor(undefined)
  }, [])

  return {
    cursor,
    limit,
    sortOrder,
    hasNext,
    hasPrev,
    nextCursor,
    prevCursor,
    setCursor,
    setLimit: updateLimit,
    setSortOrder: updateSortOrder,
    nextPage,
    previousPage,
    goToFirstPage,
    updatePaginationState,
    reset
  }
}

export type UseCursorPaginationReturn = ReturnType<typeof useCursorPagination>
