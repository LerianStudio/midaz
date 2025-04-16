import { renderHook, act } from '@testing-library/react'
import { useIntl } from 'react-intl'
import { useTransactionFormErrors } from './use-transaction-form-errors'
import { TransactionFormSchema } from './schemas'

jest.mock('react-intl', () => ({
  useIntl: jest.fn()
}))

describe('useTransactionFormErrors', () => {
  const intlMock = {
    formatMessage: jest.fn(({ defaultMessage }) => defaultMessage)
  }

  beforeEach(() => {
    ;(useIntl as jest.Mock).mockReturnValue(intlMock)
  })

  it('should return no errors initially', () => {
    const { result } = renderHook(() =>
      useTransactionFormErrors({
        value: 0,
        source: [],
        destination: []
      } as any)
    )
    expect(result.current.errors).toEqual({})
  })

  it('should add debit error if source sum does not match value', () => {
    const { result } = renderHook(() =>
      useTransactionFormErrors({
        value: 100,
        source: [{ value: 50 }],
        destination: []
      } as any)
    )

    act(() => {
      result.current.errors
    })

    expect(result.current.errors.debit).toBe(
      'Total Debits do not match total Credits'
    )
  })

  it('should add credit error if destination sum does not match value', () => {
    const { result } = renderHook(() =>
      useTransactionFormErrors({
        value: '100',
        source: [],
        destination: [{ value: '50' }]
      } as any)
    )

    act(() => {
      result.current.errors
    })

    expect(result.current.errors.credit).toBe(
      'Total Debits do not match total Credits'
    )
  })

  it('should remove debit error if source sum matches value', () => {
    const { result } = renderHook(() =>
      useTransactionFormErrors({
        value: '100',
        source: [{ value: '100' }],
        destination: []
      } as any)
    )

    act(() => {
      result.current.errors
    })

    expect(result.current.errors.debit).toBeUndefined()
  })

  it('should remove credit error if destination sum matches value', () => {
    const { result } = renderHook(() =>
      useTransactionFormErrors({
        value: '100',
        source: [],
        destination: [{ value: '100' }]
      } as any)
    )

    act(() => {
      result.current.errors
    })

    expect(result.current.errors.credit).toBeUndefined()
  })
})
