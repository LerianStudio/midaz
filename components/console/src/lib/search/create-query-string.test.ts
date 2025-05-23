import { createQueryString } from './create-query-string'

describe('createQueryString', () => {
  it('should return an empty string if data is null or undefined', () => {
    expect(createQueryString(null as any)).toBe('')
    expect(createQueryString(undefined)).toBe('')
  })

  it('should return an empty string if data is an empty object', () => {
    expect(createQueryString({})).toBe('')
  })

  it('should remove null, undefined, and empty string values from the query string', () => {
    const data = {
      param1: 'value1',
      param2: null,
      param3: undefined,
      param4: '',
      param5: 'value5'
    }
    expect(createQueryString(data)).toBe('?param1=value1&param5=value5')
  })

  it('should return a query string with valid parameters', () => {
    const data = {
      param1: 'value1',
      param2: 'value2'
    }
    expect(createQueryString(data)).toBe('?param1=value1&param2=value2')
  })

  it('should handle special characters in the query string', () => {
    const data = {
      param1: 'value with spaces',
      param2: 'value&with&special&chars'
    }
    expect(createQueryString(data)).toBe(
      '?param1=value+with+spaces&param2=value%26with%26special%26chars'
    )
  })

  it('should return an empty string if all values are null, undefined, or empty', () => {
    const data = {
      param1: null,
      param2: undefined,
      param3: ''
    }
    expect(createQueryString(data)).toBe('')
  })
})
