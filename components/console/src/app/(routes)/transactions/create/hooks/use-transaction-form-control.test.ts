import { renderHook, act } from '@testing-library/react'
import { useTransactionFormControl } from './use-transaction-form-control'

jest.mock('../../../../../hooks/use-stepper', () => ({
  useStepper: jest.fn()
}))

jest.mock('../../../../../hooks/use-transaction-routes-config', () => ({
  useTransactionRoutesConfig: jest.fn(() => ({
    shouldUseRoutes: false,
    transactionRoutes: [],
    isLoading: false,
    error: null
  }))
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
    require('../../../../../hooks/use-stepper').useStepper.mockImplementation(
      () => ({
        ...useStepperMock,
        step: 2
      })
    )
    renderHook(() =>
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

  describe('with transaction routes enabled', () => {
    beforeEach(() => {
      jest.clearAllMocks()
      require('../../../../../hooks/use-transaction-routes-config').useTransactionRoutesConfig.mockReturnValue(
        {
          shouldUseRoutes: true,
          transactionRoutes: [{ id: 'route1', title: 'Route 1' }],
          isLoading: false,
          error: null
        }
      )
    })

    it('should disable next if transactionRoute is not selected when routes are enabled', () => {
      const { result } = renderHook(() =>
        useTransactionFormControl({
          asset: 'BTC',
          value: 100,
          source: [],
          destination: [],
          transactionRoute: '',
          metadata: {}
        } as any)
      )
      expect(result.current.enableNext).toBe(false)
    })

    it('should enable next if transactionRoute is selected when routes are enabled', () => {
      const { result } = renderHook(() =>
        useTransactionFormControl({
          asset: 'BTC',
          value: 100,
          source: [],
          destination: [],
          transactionRoute: 'route1',
          metadata: {}
        } as any)
      )
      expect(result.current.enableNext).toBe(true)
    })
  })
})
