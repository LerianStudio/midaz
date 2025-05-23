import { renderHook } from '@testing-library/react'
import { useDebounce } from './use-debounce'

jest.useFakeTimers()

describe('useDebounce', () => {
  it('should call the callback after the specified delay', () => {
    const callback = jest.fn()
    const { rerender } = renderHook(
      ({ deps }: { deps: number[] }) => useDebounce(callback, 500, deps),
      {
        initialProps: { deps: [1] }
      }
    )

    expect(callback).not.toHaveBeenCalled()

    jest.advanceTimersByTime(500)

    expect(callback).toHaveBeenCalledTimes(1)

    rerender({ deps: [2] })

    jest.advanceTimersByTime(500)

    expect(callback).toHaveBeenCalledTimes(2)
  })

  it('should reset the timer when dependencies change', () => {
    const callback = jest.fn()
    const { rerender } = renderHook(
      ({ deps }) => useDebounce(callback, 500, deps),
      {
        initialProps: { deps: [1] }
      }
    )

    jest.advanceTimersByTime(300)
    rerender({ deps: [2] })

    jest.advanceTimersByTime(300)

    expect(callback).not.toHaveBeenCalled()

    jest.advanceTimersByTime(200)

    expect(callback).toHaveBeenCalledTimes(1)
  })

  it('should clean up the timer on unmount', () => {
    const callback = jest.fn()
    const { unmount } = renderHook(() => useDebounce(callback, 500, []))

    unmount()

    jest.advanceTimersByTime(500)

    expect(callback).not.toHaveBeenCalled()
  })
})
