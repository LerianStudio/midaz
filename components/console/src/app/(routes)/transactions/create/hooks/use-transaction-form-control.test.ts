import { renderHook, act } from '@testing-library/react'
import { useTransactionFormControl } from './use-transaction-form-control'

// Mock useStepper hook
jest.mock('../../../../../hooks/use-stepper', () => ({
  useStepper: jest.fn()
}))

const mockSetStep = jest.fn()
const mockHandlePrevious = jest.fn()
const mockHandleNext = jest.fn()

const useStepperMock = {
  step: 0,
  setStep: mockSetStep,
  handlePrevious: mockHandlePrevious,
  handleNext: mockHandleNext
}

beforeEach(() => {
  jest.clearAllMocks()
  // @ts-ignore
  require('../../../../../hooks/use-stepper').useStepper.mockImplementation(
    () => ({
      ...useStepperMock
    })
  )
})

describe('useTransactionFormControl', () => {
  it('should disable next if asset is empty and value is 0', () => {
    const { result } = renderHook(() =>
      useTransactionFormControl({
        asset: '',
        value: 0,
        source: [],
        destination: [],
        metadata: {}
      } as any)
    )
    expect(result.current.enableNext).toBe(false)
  })

  it('should enable next if asset is filled and value > 0', () => {
    const { result, rerender } = renderHook(
      ({ asset, value }) =>
        useTransactionFormControl({
          asset,
          value,
          source: '',
          destination: ''
        } as any),
      { initialProps: { asset: 'BTC', value: 100 } }
    )
    expect(result.current.enableNext).toBe(true)

    rerender({ asset: '', value: 100 })
    expect(result.current.enableNext).toBe(false)
  })

  it('should enable next on step 1 if source and destination are filled', () => {
    // @ts-ignore
    require('../../../../../hooks/use-stepper').useStepper.mockImplementation(
      () => ({
        ...useStepperMock,
        step: 1
      })
    )
    const { result } = renderHook(() =>
      useTransactionFormControl({
        asset: 'BTC',
        value: 100,
        source: [{ account: 'test' }],
        destination: [{ account: 'test2' }]
      } as any)
    )
    expect(result.current.enableNext).toBe(true)
  })

  it('should disable next on step 1 if source or destination is empty', () => {
    // @ts-ignore
    require('../../../../../hooks/use-stepper').useStepper.mockImplementation(
      () => ({
        ...useStepperMock,
        step: 1
      })
    )
    const { result } = renderHook(() =>
      useTransactionFormControl({
        asset: 'BTC',
        value: 100,
        source: [],
        destination: [{ account: 'test2' }]
      } as any)
    )
    expect(result.current.enableNext).toBe(false)
  })

  it('should call handlePrevious and not enable next on step 2 if source or destination is empty', () => {
    // @ts-ignore
    require('../../../../../hooks/use-stepper').useStepper.mockImplementation(
      () => ({
        ...useStepperMock,
        step: 2
      })
    )
    const { result } = renderHook(() =>
      useTransactionFormControl({
        asset: 'BTC',
        value: 100,
        source: [],
        destination: [{ account: 'test2' }]
      } as any)
    )
    expect(mockHandlePrevious).toHaveBeenCalled()
  })

  it('should enable next on step 2 if source and destination are filled', () => {
    // @ts-ignore
    require('../../../../../hooks/use-stepper').useStepper.mockImplementation(
      () => ({
        ...useStepperMock,
        step: 2
      })
    )
    const { result } = renderHook(() =>
      useTransactionFormControl({
        asset: 'BTC',
        value: 100,
        source: [{ account: 'test' }],
        destination: [{ account: 'test2' }]
      } as any)
    )
    expect(result.current.enableNext).toBe(true)
  })

  it('should call _handleNext and reset enableNext when handleNext is called and enableNext is true', () => {
    const { result } = renderHook(() =>
      useTransactionFormControl({
        asset: 'BTC',
        value: 100,
        source: [{ account: 'test' }],
        destination: [{ account: 'test2' }]
      } as any)
    )
    // enableNext is true on step 0 with asset and value filled
    act(() => {
      result.current.handleNext()
    })
    expect(mockHandleNext).toHaveBeenCalled()
  })

  it('should not call _handleNext if enableNext is false', () => {
    const { result } = renderHook(() =>
      useTransactionFormControl({
        asset: '',
        value: 0,
        source: [],
        destination: []
      } as any)
    )
    act(() => {
      result.current.handleNext()
    })
    expect(mockHandleNext).not.toHaveBeenCalled()
  })
})
