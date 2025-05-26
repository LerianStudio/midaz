import { renderHook, act } from '@testing-library/react'
import { normalize, useNormalize } from './use-normalize'

describe('normalize', () => {
  it('should normalize array of objects by given key', () => {
    const arr = [
      { id: 'a', value: 1 },
      { id: 'b', value: 2 }
    ]
    const result = normalize(arr, 'id')
    expect(result).toEqual({
      a: { id: 'a', value: 1 },
      b: { id: 'b', value: 2 }
    })
  })

  it('should return empty object for empty array', () => {
    expect(normalize([], 'id')).toEqual({})
  })

  it('should overwrite duplicate keys', () => {
    const arr = [
      { id: 'a', value: 1 },
      { id: 'a', value: 2 }
    ]
    const result = normalize(arr, 'id')
    expect(result).toEqual({
      a: { id: 'a', value: 2 }
    })
  })
})

describe('useNormalize', () => {
  it('should initialize with empty object if no value is provided', () => {
    const { result } = renderHook(() => useNormalize())
    expect(result.current.data).toEqual({})
  })

  it('should initialize with provided value', () => {
    const initial = { a: { id: 'a', value: 1 } }
    const { result } = renderHook(() => useNormalize(initial))
    expect(result.current.data).toEqual(initial)
  })

  it('should add an item', () => {
    const { result } = renderHook(() => useNormalize())
    act(() => {
      result.current.add('b', { id: 'b', value: 2 })
    })
    expect(result.current.data).toEqual({
      b: { id: 'b', value: 2 }
    })
  })

  it('should remove an item', () => {
    const initial = { a: { id: 'a', value: 1 }, b: { id: 'b', value: 2 } }
    const { result } = renderHook(() => useNormalize(initial))
    act(() => {
      result.current.remove('a')
    })
    expect(result.current.data).toEqual({ b: { id: 'b', value: 2 } })
  })

  it('should clear all items', () => {
    const initial = { a: { id: 'a', value: 1 } }
    const { result } = renderHook(() => useNormalize(initial))
    act(() => {
      result.current.clear()
    })
    expect(result.current.data).toEqual({})
  })

  it('should set data directly', () => {
    const { result } = renderHook(() => useNormalize())
    act(() => {
      result.current.set({ x: { id: 'x', value: 10 } })
    })
    expect(result.current.data).toEqual({ x: { id: 'x', value: 10 } })
  })
})
