import { renderHook } from '@testing-library/react'
import { useFormatAmount } from './use-format-amount'
import { useFormatNumber } from '@/lib/intl/use-format-number'
import { useLocale } from '@/lib/intl/use-locale'
import { transactions as lib } from '@lerian/lib-commons-js'

// Mock dependencies
jest.mock('../lib/intl/use-format-number', () => ({
  useFormatNumber: jest.fn()
}))

jest.mock('../lib/intl/use-locale', () => ({
  useLocale: jest.fn()
}))

jest.mock('@lerian/lib-commons-js', () => ({
  transactions: {
    undoScaleDecimal: jest.fn()
  }
}))

describe('useFormatAmount', () => {
  // Setup common mocks
  const mockFormatNumber = jest.fn((value) => `formatted:${value}`)
  const mockUndoScaleDecimal = jest.fn(
    (value, scale) => value * Math.pow(10, scale)
  )

  beforeEach(() => {
    jest.clearAllMocks()
    ;(useFormatNumber as jest.Mock).mockReturnValue({
      formatNumber: mockFormatNumber
    })
    ;(useLocale as jest.Mock).mockReturnValue({ locale: 'en-US' })
    ;(lib.undoScaleDecimal as jest.Mock).mockImplementation(
      mockUndoScaleDecimal
    )
  })

  test('should format amount correctly', () => {
    // Arrange
    const amount = { value: 10000, scale: 2 }

    // Act
    const { result } = renderHook(() => useFormatAmount())
    const formattedAmount = result.current.formatAmount(amount)

    // Assert
    expect(lib.undoScaleDecimal).toHaveBeenCalledWith(10000, -2)
    expect(mockFormatNumber).toHaveBeenCalledWith(100)
    expect(formattedAmount).toBe('formatted:100')
  })

  test('should handle zero amount', () => {
    // Arrange
    const amount = { value: 0, scale: 2 }

    // Act
    const { result } = renderHook(() => useFormatAmount())
    const formattedAmount = result.current.formatAmount(amount)

    // Assert
    expect(lib.undoScaleDecimal).toHaveBeenCalledWith(0, -2)
    expect(mockFormatNumber).toHaveBeenCalledWith(0)
    expect(formattedAmount).toBe('formatted:0')
  })

  test('should handle negative amounts', () => {
    // Arrange
    const amount = { value: -5000, scale: 2 }

    // Act
    const { result } = renderHook(() => useFormatAmount())
    const formattedAmount = result.current.formatAmount(amount)

    // Assert
    expect(lib.undoScaleDecimal).toHaveBeenCalledWith(-5000, -2)
    expect(mockFormatNumber).toHaveBeenCalledWith(-50)
    expect(formattedAmount).toBe('formatted:-50')
  })

  test('should handle different scales', () => {
    // Arrange
    const amount = { value: 10000, scale: 3 }

    // Act
    const { result } = renderHook(() => useFormatAmount())
    const formattedAmount = result.current.formatAmount(amount)

    // Assert
    expect(lib.undoScaleDecimal).toHaveBeenCalledWith(10000, -3)
    expect(mockFormatNumber).toHaveBeenCalledWith(10)
    expect(formattedAmount).toBe('formatted:10')
  })

  test('should update when locale changes', () => {
    // Arrange
    const amount = { value: 10000, scale: 2 }

    // First render with en-US locale
    const { result, rerender } = renderHook(() => useFormatAmount())
    result.current.formatAmount(amount)

    // Change locale and rerender
    ;(useLocale as jest.Mock).mockReturnValue({ locale: 'fr-FR' })
    rerender()

    // Act
    const formattedAmount = result.current.formatAmount(amount)

    // Assert
    expect(formattedAmount).toBe('formatted:100')
    expect(mockFormatNumber).toHaveBeenCalledTimes(2)
  })
})
