import { renderHook, act } from '@testing-library/react'
import { usePagination } from './use-pagination'

describe('usePagination', () => {
  it('should initialize with default values', () => {
    const { result } = renderHook(() => usePagination({ total: 100 }))
    expect(result.current.page).toBe(1)
    expect(result.current.limit).toBe(10)
  })

  it('should go to the next page', () => {
    const { result } = renderHook(() => usePagination({ total: 100 }))
    act(() => {
      result.current.nextPage()
    })
    expect(result.current.page).toBe(2)
  })

  it('should not go to the next page if on the last page', () => {
    const { result } = renderHook(() => usePagination({ total: 10 }))
    act(() => {
      result.current.nextPage()
      result.current.nextPage()
    })
    expect(result.current.page).toBe(1)
  })

  it('should go to the previous page', () => {
    const { result } = renderHook(() => usePagination({ total: 100 }))
    act(() => {
      result.current.nextPage()
    })
    act(() => {
      result.current.previousPage()
    })
    expect(result.current.page).toBe(1)
  })

  it('should not go to the previous page if on the first page', () => {
    const { result } = renderHook(() => usePagination({ total: 100 }))
    act(() => {
      result.current.previousPage()
    })
    expect(result.current.page).toBe(1)
  })

  it('should set the page correctly', () => {
    const { result } = renderHook(() => usePagination({ total: 100 }))
    act(() => {
      result.current.setPage(5)
    })
    expect(result.current.page).toBe(5)
  })

  it('should not set the page if out of range', () => {
    const { result } = renderHook(() => usePagination({ total: 100 }))
    act(() => {
      result.current.setPage(11)
    })
    expect(result.current.page).toBe(1)
  })

  it('should set the limit correctly', () => {
    const { result } = renderHook(() => usePagination({ total: 100 }))
    act(() => {
      result.current.setLimit(20)
    })
    expect(result.current.limit).toBe(20)
  })

  it('should not set the limit if less than 1', () => {
    const { result } = renderHook(() => usePagination({ total: 100 }))
    act(() => {
      result.current.setLimit(0)
    })
    expect(result.current.limit).toBe(10)
  })
})
