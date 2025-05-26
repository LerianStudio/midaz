import { getInitialValues } from './get-initial-values'

describe('getInitialValues', () => {
  it('should return an empty object if initialValues is undefined', () => {
    const result = getInitialValues(undefined, { name: 'John' })
    expect(result).toEqual({})
  })

  it('should return initialValues if data is undefined', () => {
    const initialValues = { name: '' }
    const result = getInitialValues(initialValues, undefined)
    expect(result).toEqual(initialValues)
  })

  it('should merge initialValues with data', () => {
    const initialValues = { name: '', age: 0 }
    const data = { name: 'Doe', age: 25, city: 'New York' }
    const result = getInitialValues(initialValues, data)
    expect(result).toEqual({ name: 'Doe', age: 25 })
  })

  it('should not include keys from data that are not in initialValues', () => {
    const initialValues = { name: '' }
    const data = { name: 'Doe', age: 25 }
    const result = getInitialValues(initialValues, data)
    expect(result).toEqual({ name: 'Doe' })
  })

  it('should return initialValues if data does not contain any matching keys', () => {
    const initialValues = { name: '' }
    const data = { age: 25, city: 'New York' }
    const result = getInitialValues(initialValues, data)
    expect(result).toEqual(initialValues)
  })
})
