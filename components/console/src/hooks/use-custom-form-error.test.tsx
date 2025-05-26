import { renderHook, act } from '@testing-library/react'
import { useCustomFormError } from './use-custom-form-error'

describe('useCustomFormError', () => {
  it('should initialize with no error', () => {
    const { result } = renderHook(() => useCustomFormError())
    expect(result.current.errors).toEqual({})
  })

  it('should add an error', () => {
    const { result } = renderHook(() => useCustomFormError())
    act(() => {
      result.current.add('test-error', { message: 'Test error' })
    })
    expect(result.current.errors).toEqual({
      'test-error': { message: 'Test error' }
    })
  })

  it('should clear the error', () => {
    const { result } = renderHook(() => useCustomFormError())
    act(() => {
      result.current.add('test-error', { message: 'Test error' })
    })
    act(() => {
      result.current.remove('test-error')
    })
    expect(result.current.errors).toEqual({})
  })

  it('should overwrite previous error', () => {
    const { result } = renderHook(() => useCustomFormError())
    act(() => {
      result.current.add('test-error', { message: 'Test error' })
    })
    act(() => {
      result.current.add('test-error', { message: 'Test error 2' })
    })
    expect(result.current.errors).toEqual({
      'test-error': { message: 'Test error 2' }
    })
  })

  it('should clear all errors', () => {
    const { result } = renderHook(() => useCustomFormError())
    act(() => {
      result.current.add('test-error', { message: 'Test error' })
    })
    act(() => {
      result.current.add('test-error-2', { message: 'Test error' })
    })
    act(() => {
      result.current.clear()
    })
    expect(result.current.errors).toEqual({})
  })
})
