import { renderHook } from '@testing-library/react'
import Decimal from 'decimal.js-light'
import { useFormatNumber } from './use-format-number'
import { useLocale } from './use-locale'

// Mock useLocale
jest.mock('./use-locale', () => ({
  useLocale: jest.fn()
}))

describe('useFormatNumber', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  it('should use dot as separator for en-US locale', () => {
    ;(useLocale as jest.Mock).mockReturnValue({ locale: 'en-US' })

    const mockFormatToParts = jest.fn().mockReturnValue([
      { type: 'integer', value: '1' },
      { type: 'decimal', value: '.' },
      { type: 'fraction', value: '1' }
    ])

    const originalIntl = global.Intl
    global.Intl = {
      ...originalIntl,
      NumberFormat: jest.fn().mockImplementation(() => ({
        formatToParts: mockFormatToParts
      }))
    } as any

    const { result } = renderHook(() => useFormatNumber())
    expect(result.current.formatNumber(new Decimal('123.45'))).toBe('123.45')

    global.Intl = originalIntl
  })

  it('should use comma as separator for de-DE locale', () => {
    ;(useLocale as jest.Mock).mockReturnValue({ locale: 'de-DE' })

    const mockFormatToParts = jest.fn().mockReturnValue([
      { type: 'integer', value: '1' },
      { type: 'decimal', value: ',' },
      { type: 'fraction', value: '1' }
    ])

    const originalIntl = global.Intl
    global.Intl = {
      ...originalIntl,
      NumberFormat: jest.fn().mockImplementation(() => ({
        formatToParts: mockFormatToParts
      }))
    } as any

    const { result } = renderHook(() => useFormatNumber())
    expect(result.current.formatNumber(new Decimal('123.45'))).toBe('123,45')

    global.Intl = originalIntl
  })

  it('should use default separator when decimal part is not found', () => {
    ;(useLocale as jest.Mock).mockReturnValue({ locale: 'en-US' })

    const mockFormatToParts = jest
      .fn()
      .mockReturnValue([{ type: 'integer', value: '1' }])

    const originalIntl = global.Intl
    global.Intl = {
      ...originalIntl,
      NumberFormat: jest.fn().mockImplementation(() => ({
        formatToParts: mockFormatToParts
      }))
    } as any

    const { result } = renderHook(() => useFormatNumber())
    expect(result.current.formatNumber(new Decimal('123.45'))).toBe('123.45')

    global.Intl = originalIntl
  })

  it('should handle various number formats correctly', () => {
    ;(useLocale as jest.Mock).mockReturnValue({ locale: 'en-US' })

    const mockFormatToParts = jest.fn().mockReturnValue([
      { type: 'integer', value: '1' },
      { type: 'decimal', value: '.' },
      { type: 'fraction', value: '1' }
    ])

    const originalIntl = global.Intl
    global.Intl = {
      ...originalIntl,
      NumberFormat: jest.fn().mockImplementation(() => ({
        formatToParts: mockFormatToParts
      }))
    } as any

    const { result } = renderHook(() => useFormatNumber())

    expect(result.current.formatNumber(new Decimal('0'))).toBe('0')
    expect(result.current.formatNumber(new Decimal('123'))).toBe('123')
    expect(result.current.formatNumber(new Decimal('0.123'))).toBe('0.123')
    expect(result.current.formatNumber(new Decimal('1000000.123456'))).toBe(
      '1000000.123456'
    )

    global.Intl = originalIntl
  })
})
