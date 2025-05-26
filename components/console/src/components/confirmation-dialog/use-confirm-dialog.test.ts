import { renderHook, act } from '@testing-library/react'
import { useConfirmDialog } from './use-confirm-dialog'

describe('useConfirmDialog', () => {
  it('should initialize with default values', () => {
    const { result } = renderHook(() => useConfirmDialog({}))

    expect(result.current.id).toBe('')
    expect(result.current.data).toBeNull()
    expect(result.current.dialogProps.open).toBe(false)
  })

  it('should open dialog with correct id and data', () => {
    const { result } = renderHook(() => useConfirmDialog({}))

    act(() => {
      result.current.handleDialogOpen('123', { name: 'test' })
    })

    expect(result.current.id).toBe('123')
    expect(result.current.data).toEqual({ name: 'test' })
    expect(result.current.dialogProps.open).toBe(true)
  })

  it('should close dialog on cancel', () => {
    const { result } = renderHook(() => useConfirmDialog({}))

    act(() => {
      result.current.handleDialogOpen('123', { name: 'test' })
    })

    act(() => {
      result.current.dialogProps.onCancel()
    })

    expect(result.current.id).toBe('')
    expect(result.current.data).toBeNull()
    expect(result.current.dialogProps.open).toBe(false)
  })

  it('should call onConfirmProp and stay open on confirm', () => {
    const onConfirmMock = jest.fn()
    const { result } = renderHook(() =>
      useConfirmDialog({ onConfirm: onConfirmMock })
    )

    act(() => {
      result.current.handleDialogOpen('123', { name: 'test' })
    })

    act(() => {
      result.current.dialogProps.onConfirm()
    })

    expect(onConfirmMock).toHaveBeenCalledWith('123')
    expect(result.current.id).toBe('')
    expect(result.current.data).toBeNull()
    expect(result.current.dialogProps.open).toBe(true)
  })
})
