import { getStorageObject } from './get-storage-object'
import { getStorage } from './get-storage'

jest.mock('./get-storage')

describe('getStorageObject', () => {
  const key = 'testKey'
  const defaultValue = { default: 'value' }

  beforeEach(() => {
    jest.clearAllMocks()
  })

  it('should return parsed object when getStorage returns valid JSON string', () => {
    const dataString = '{"test": "value"}'
    ;(getStorage as jest.Mock).mockReturnValue(dataString)

    const result = getStorageObject(key, defaultValue)

    expect(result).toEqual({ test: 'value' })
    expect(getStorage).toHaveBeenCalledWith(key, defaultValue)
  })

  it('should return default value when getStorage throws an error', () => {
    ;(getStorage as jest.Mock).mockImplementation(() => {
      throw new Error('Storage error')
    })

    const result = getStorageObject(key, defaultValue)

    expect(result).toEqual(defaultValue)
    expect(getStorage).toHaveBeenCalledWith(key, defaultValue)
  })

  it('should return default value when JSON.parse throws an error', () => {
    const invalidJsonString = 'invalid JSON'
    ;(getStorage as jest.Mock).mockReturnValue(invalidJsonString)

    const result = getStorageObject(key, defaultValue)

    expect(result).toEqual(defaultValue)
    expect(getStorage).toHaveBeenCalledWith(key, defaultValue)
  })
})
