import { renderHook, act } from '@testing-library/react'
import { useSearchParams } from './use-search-params'
import {
  useRouter,
  usePathname,
  useSearchParams as useNextSearchParams
} from 'next/navigation'
import { createQueryString } from './create-query-string'
import { getSearchParams } from './get-search-params'

jest.mock('next/navigation', () => ({
  useRouter: jest.fn(),
  usePathname: jest.fn(),
  useSearchParams: jest.fn()
}))

jest.mock('./create-query-string', () => ({
  createQueryString: jest.fn()
}))

jest.mock('./get-search-params', () => ({
  getSearchParams: jest.fn()
}))

describe('useSearchParams', () => {
  const mockReplace = jest.fn()
  const mockPathname = '/test-path'
  const mockSearchParams = { param1: 'value1', param2: 'value2' }

  beforeEach(() => {
    ;(useRouter as jest.Mock).mockReturnValue({ replace: mockReplace })
    ;(usePathname as jest.Mock).mockReturnValue(mockPathname)
    ;(useNextSearchParams as jest.Mock).mockReturnValue(mockSearchParams)
    ;(getSearchParams as jest.Mock).mockReturnValue(mockSearchParams)
    jest.clearAllMocks()
  })

  it('should set new search params', () => {
    const { result } = renderHook(() => useSearchParams())
    const newParams = { param3: 'value3' }
    const expectedQueryString = '?param3=value3'

    ;(createQueryString as jest.Mock).mockReturnValue(expectedQueryString)

    act(() => {
      result.current.setSearchParams(newParams)
    })

    expect(createQueryString).toHaveBeenCalledWith(newParams)
    expect(mockReplace).toHaveBeenCalledWith(mockPathname + expectedQueryString)
  })

  it('should update search params', () => {
    const { result } = renderHook(() => useSearchParams())
    const updatedParams = { param2: 'newValue2', param3: 'value3' }
    const expectedQueryString = '?param1=value1&param2=newValue2&param3=value3'

    ;(createQueryString as jest.Mock).mockReturnValue(expectedQueryString)

    act(() => {
      result.current.updateSearchParams(updatedParams)
    })

    expect(createQueryString).toHaveBeenCalledWith({
      param1: 'value1',
      param2: 'newValue2',
      param3: 'value3'
    })
    expect(mockReplace).toHaveBeenCalledWith(mockPathname + expectedQueryString)
  })
})
