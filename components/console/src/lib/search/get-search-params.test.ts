import { getSearchParams } from './get-search-params'

describe('getSearchParams', () => {
  beforeEach(() => {
    // Mock window.location.search
    Object.defineProperty(window, 'location', {
      writable: true,
      value: { search: '' }
    })
  })

  it('should return an empty object when there are no search params', () => {
    window.location.search = ''
    const result = getSearchParams()
    expect(result).toEqual({})
  })

  it('should return an object with a single search param', () => {
    window.location.search = '?key=value'
    const result = getSearchParams()
    expect(result).toEqual({ key: 'value' })
  })

  it('should return an object with multiple search params', () => {
    window.location.search = '?key1=value1&key2=value2'
    const result = getSearchParams()
    expect(result).toEqual({ key1: 'value1', key2: 'value2' })
  })

  it('should handle search params with empty values', () => {
    window.location.search = '?key1=&key2=value2'
    const result = getSearchParams()
    expect(result).toEqual({ key1: '', key2: 'value2' })
  })

  it('should decode encoded search params', () => {
    window.location.search = '?key1=value%201&key2=value%202'
    const result = getSearchParams()
    expect(result).toEqual({ key1: 'value 1', key2: 'value 2' })
  })
})
