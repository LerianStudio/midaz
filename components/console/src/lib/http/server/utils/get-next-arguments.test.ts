import {
  getNextRequestArgument,
  getNextParamArgument
} from './get-next-arguments'

describe('getNextRequestArgument', () => {
  it('returns the first argument when present', () => {
    const req = { foo: 'bar' }
    expect(getNextRequestArgument([req, { params: {} }])).toBe(req)
  })

  it('returns undefined if args is empty', () => {
    expect(getNextRequestArgument([])).toBeUndefined()
  })

  it('returns undefined if args is not provided', () => {
    // @ts-expect-error testing missing argument
    expect(getNextRequestArgument()).toBeUndefined()
  })

  it('returns null if first argument is null', () => {
    expect(getNextRequestArgument([null, { params: {} }])).toBeNull()
  })
})

describe('getNextParamArgument', () => {
  it('returns params property of second argument', () => {
    const params = { id: 123 }
    expect(getNextParamArgument([{}, { params }])).toBe(params)
  })

  it('throws if second argument is missing', () => {
    expect(getNextParamArgument([{}])).toBeUndefined()
  })

  it('throws if second argument has no params property', () => {
    expect(getNextParamArgument([{}, {}])).toBeUndefined()
  })

  it('returns undefined if params is undefined', () => {
    expect(getNextParamArgument([{}, { params: undefined }])).toBeUndefined()
  })

  it('returns null if params is null', () => {
    expect(getNextParamArgument([{}, { params: null }])).toBeUndefined()
  })
})
