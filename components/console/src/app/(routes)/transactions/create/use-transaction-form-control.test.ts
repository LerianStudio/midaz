import { renderHook, act } from '@testing-library/react'
import { useTransactionFormControl } from './use-transaction-form-control'
import { TransactionFormSchema } from './schemas'

describe('useTransactionFormControl', () => {
  const initialValues: TransactionFormSchema = {
    asset: '',
    value: 0,
    source: [],
    destination: []
  } as any

  it('should initialize with step 0', () => {
    const { result } = renderHook(() =>
      useTransactionFormControl(initialValues)
    )
    expect(result.current.step).toBe(0)
  })

  it('should set step to 1 when value, asset, source, and destination are provided', () => {
    const values: TransactionFormSchema = {
      asset: 'BTC',
      value: 100,
      source: [{ account: 'source1' }],
      destination: [{ account: 'destination1' }]
    } as any

    const { result } = renderHook(() => useTransactionFormControl(values))

    act(() => {
      result.current.step
    })

    expect(result.current.step).toBe(1)
  })

  it('should set step to 0 when any of value, asset, source, or destination are missing', () => {
    const values: TransactionFormSchema = {
      asset: '',
      value: 100,
      source: [{ account: 'source1' }],
      destination: [{ account: 'destination1' }]
    } as any

    const { result } = renderHook(() => useTransactionFormControl(values))

    act(() => {
      result.current.step
    })

    expect(result.current.step).toBe(0)
  })

  it('should not change step if step is 2 or more', () => {
    const values: TransactionFormSchema = {
      asset: 'BTC',
      value: 100,
      source: [{ account: 'source1' }],
      destination: [{ account: 'destination1' }]
    } as any

    const { result } = renderHook(() => useTransactionFormControl(values))

    act(() => {
      result.current.setStep(2)
    })

    expect(result.current.step).toBe(2)
  })
})
