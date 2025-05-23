import { renderHook, act } from '@testing-library/react'
import { useCreateUpdateSheet } from './use-create-update-sheet'
import { useSearchParams } from '@/lib/search'

jest.mock('./../../lib/search', () => ({
  useSearchParams: jest.fn()
}))

describe('useCreateUpdateSheet', () => {
  let mockSearchParams: Record<string, string>
  let mockSetSearchParams: jest.Mock

  beforeEach(() => {
    mockSearchParams = {}
    mockSetSearchParams = jest.fn()
    ;(useSearchParams as jest.Mock).mockReturnValue({
      searchParams: mockSearchParams,
      setSearchParams: mockSetSearchParams
    })
  })

  it('should initialize with default values', () => {
    const { result } = renderHook(() => useCreateUpdateSheet())

    expect(result.current.mode).toBe('create')
    expect(result.current.data).toBeNull()
    expect(result.current.sheetProps.open).toBe(false)
  })

  it('should handle create mode', () => {
    const { result } = renderHook(() => useCreateUpdateSheet())

    act(() => {
      result.current.handleCreate()
    })

    expect(result.current.mode).toBe('create')
    expect(result.current.data).toBeNull()
    expect(result.current.sheetProps.open).toBe(true)
  })

  it('should handle edit mode', () => {
    const { result } = renderHook(() => useCreateUpdateSheet<{ id: number }>())

    const mockData = { id: 1 }
    act(() => {
      result.current.handleEdit(mockData)
    })

    expect(result.current.mode).toBe('edit')
    expect(result.current.data).toEqual(mockData)
    expect(result.current.sheetProps.open).toBe(true)
  })

  it('should close the sheet and clear URL params when onOpenChange is called with false', () => {
    mockSearchParams['create'] = 'true'
    const { result } = renderHook(() =>
      useCreateUpdateSheet({ enableRouting: true })
    )

    act(() => {
      result.current.sheetProps.onOpenChange(false)
    })

    expect(result.current.sheetProps.open).toBe(false)
    expect(mockSetSearchParams).toHaveBeenCalledWith({})
  })

  it('should open the sheet when URL contains "create=true" and enableRouting is true', () => {
    mockSearchParams['create'] = 'true'
    const { result } = renderHook(() =>
      useCreateUpdateSheet({ enableRouting: true })
    )

    expect(result.current.sheetProps.open).toBe(true)
    expect(result.current.mode).toBe('create')
  })

  it('should not open the sheet when enableRouting is false', () => {
    mockSearchParams['create'] = 'true'
    const { result } = renderHook(() =>
      useCreateUpdateSheet({ enableRouting: false })
    )

    expect(result.current.sheetProps.open).toBe(false)
  })
})
