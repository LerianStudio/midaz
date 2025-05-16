import { renderHook, act } from '@testing-library/react'
import { useTime } from './use-time'

jest.useFakeTimers()

describe('useTime', () => {
  it('should initialize with the current time', () => {
    const { result } = renderHook(() => useTime({}))
    expect(result.current).toBeInstanceOf(Date)
  })

  it('should update the time at the specified interval', () => {
    const interval = 2000
    const { result } = renderHook(() => useTime({ interval }))

    const initialTime = result.current
    act(() => {
      jest.advanceTimersByTime(interval)
    })

    expect(result.current.getTime()).toBeGreaterThan(initialTime.getTime())
  })

  it('should call onUpdate callback with the updated time', () => {
    const onUpdate = jest.fn()
    const interval = 1000
    renderHook(() => useTime({ interval, onUpdate }))

    act(() => {
      jest.advanceTimersByTime(interval)
    })

    expect(onUpdate).toHaveBeenCalledTimes(1)
    expect(onUpdate).toHaveBeenCalledWith(expect.any(Date))
  })

  it('should clear the interval on unmount', () => {
    const clearIntervalSpy = jest.spyOn(global, 'clearInterval')
    const { unmount } = renderHook(() => useTime({ interval: 1000 }))

    unmount()
    expect(clearIntervalSpy).toHaveBeenCalledTimes(1)
    clearIntervalSpy.mockRestore()
  })
})
