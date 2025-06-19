import { renderHook } from '@testing-library/react'
import { useFormatNumber } from './use-format-number'
import { useLocale } from './use-locale'
import { isNumericalString } from 'framer-motion'

jest.mock('./use-locale')
jest.mock('framer-motion')

const mockUseLocale = useLocale as jest.MockedFunction<typeof useLocale>
const mockIsNumericalString = isNumericalString as jest.MockedFunction<
  typeof isNumericalString
>

describe('useFormatNumber', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  it('should format number with default locale separators', () => {
    mockUseLocale.mockReturnValue({ locale: 'en-US' } as any)
    mockIsNumericalString.mockReturnValue(true)

    const { result } = renderHook(() => useFormatNumber())

    expect(result.current.formatNumber('1234.56')).toBe('1,234.56')
  })

  it('should format number with different locale separators', () => {
    mockUseLocale.mockReturnValue({ locale: 'de-DE' } as any)
    mockIsNumericalString.mockReturnValue(true)

    const { result } = renderHook(() => useFormatNumber())

    expect(result.current.formatNumber('1234.56')).toBe('1.234,56')
  })

  it('should return original value for non-string input', () => {
    mockUseLocale.mockReturnValue({ locale: 'en-US' } as any)

    const { result } = renderHook(() => useFormatNumber())

    expect(result.current.formatNumber(123 as any)).toBe(123)
  })

  it('should return original value for non-numerical string', () => {
    mockUseLocale.mockReturnValue({ locale: 'en-US' } as any)
    mockIsNumericalString.mockReturnValue(false)

    const { result } = renderHook(() => useFormatNumber())

    expect(result.current.formatNumber('abc')).toBe('abc')
  })

  it('should format integer without decimal part', () => {
    mockUseLocale.mockReturnValue({ locale: 'en-US' } as any)
    mockIsNumericalString.mockReturnValue(true)

    const { result } = renderHook(() => useFormatNumber())

    expect(result.current.formatNumber('1234')).toBe('1,234')
  })

  it('should handle numbers with zero decimal', () => {
    mockUseLocale.mockReturnValue({ locale: 'en-US' } as any)
    mockIsNumericalString.mockReturnValue(true)

    const { result } = renderHook(() => useFormatNumber())

    expect(result.current.formatNumber('1234.0')).toBe('1,234.0')
  })

  it('should handle small numbers without thousand separator', () => {
    mockUseLocale.mockReturnValue({ locale: 'en-US' } as any)
    mockIsNumericalString.mockReturnValue(true)

    const { result } = renderHook(() => useFormatNumber())

    expect(result.current.formatNumber('123.45')).toBe('123.45')
  })
})
