import { renderHook, act } from '@testing-library/react'
import { useStepper } from './use-stepper'

describe('useStepper', () => {
  it('should initialize with the correct step', () => {
    const { result } = renderHook(() => useStepper({}))
    expect(result.current.step).toBe(0)
  })

  it('should go to the next step', () => {
    const { result } = renderHook(() => useStepper({}))
    act(() => {
      result.current.handleNext()
    })
    expect(result.current.step).toBe(1)
  })

  it('should go to the previous step', () => {
    const { result } = renderHook(() => useStepper({ defaultStep: 1 }))
    act(() => {
      result.current.handlePrevious()
    })
    expect(result.current.step).toBe(0)
  })

  it('should not go below step 0', () => {
    const { result } = renderHook(() => useStepper({}))
    act(() => {
      result.current.handlePrevious()
    })
    expect(result.current.step).toBe(0)
  })

  it('should not exceed the maximum steps', () => {
    const { result } = renderHook(() => useStepper({ maxSteps: 2 }))
    expect(result.current.step).toBe(0)
    act(() => {
      result.current.handleNext()
    })
    act(() => {
      result.current.handleNext()
    })
    act(() => {
      result.current.handleNext()
    })
    expect(result.current.step).toBe(1)
  })
})
