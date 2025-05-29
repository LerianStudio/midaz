import { renderHook, act } from '@testing-library/react'
import { useToast } from './use-toast'

describe('useToast', () => {
  beforeAll(() => {
    jest.useFakeTimers()
  })

  afterAll(() => {
    jest.useRealTimers()
  })

  it('should add a toast', () => {
    const { result } = renderHook(() => useToast())

    act(() => {
      result.current.toast({
        title: 'Test Toast',
        description: 'This is a test'
      })
    })

    expect(result.current.toasts).toHaveLength(1)
    expect(result.current.toasts[0]).toMatchObject({
      title: 'Test Toast',
      description: 'This is a test',
      open: true
    })
  })

  it('should dismiss a toast', () => {
    const { result } = renderHook(() => useToast())

    let toastId: string
    act(() => {
      const newToast = result.current.toast({ title: 'Dismiss Test' })
      toastId = newToast.id
    })

    act(() => {
      result.current.dismiss(toastId)
    })

    expect(result.current.toasts[0].open).toBe(false)
  })

  it('should update a toast', () => {
    const { result } = renderHook(() => useToast())

    let toastId: string
    act(() => {
      const newToast = result.current.toast({ title: 'Update Test' })
      toastId = newToast.id
    })

    act(() => {
      result.current.toast({ id: toastId, title: 'Updated Title' } as any)
    })

    expect(result.current.toasts[0].title).toBe('Updated Title')
  })

  it('should remove a toast after dismissing', () => {
    const { result } = renderHook(() => useToast())

    let toastId: string
    act(() => {
      const newToast = result.current.toast({ title: 'Remove Test' })
      toastId = newToast.id
    })

    act(() => {
      result.current.dismiss(toastId)
    })

    act(() => {
      jest.advanceTimersByTime(1000001) // Simulate TOAST_REMOVE_DELAY
    })

    expect(result.current.toasts).toHaveLength(0)
  })

  it('should limit the number of toasts to TOAST_LIMIT', () => {
    const { result } = renderHook(() => useToast())

    act(() => {
      result.current.toast({ title: 'Toast 1' })
      result.current.toast({ title: 'Toast 2' })
    })

    expect(result.current.toasts).toHaveLength(1) // TOAST_LIMIT is 1
    expect(result.current.toasts[0].title).toBe('Toast 2') // Only the latest toast is kept
  })
})
