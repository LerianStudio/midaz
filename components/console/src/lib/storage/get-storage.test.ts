import { getStorage } from './get-storage'

describe('getStorage', () => {
  const key = 'testKey'
  const defaultValue = 'defaultValue'

  beforeEach(() => {
    // Clear localStorage before each test
    localStorage.clear()
  })

  it('should return defaultValue if running on server', () => {
    ;(global as any).window = undefined
    expect(getStorage(key, defaultValue)).toBe(defaultValue)
  })

  it('should return defaultValue if localStorage is empty', () => {
    ;(global as any).window = {}
    expect(getStorage(key, defaultValue)).toBe(defaultValue)
  })

  it('should return the stored value if it exists in localStorage', () => {
    ;(global as any).window = {}
    localStorage.setItem(key, 'storedValue')
    expect(getStorage(key, defaultValue)).toBe('storedValue')
  })

  it('should return defaultValue if localStorage throws an error', () => {
    ;(global as any).window = {}
    jest.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('localStorage error')
    })
    expect(getStorage(key, defaultValue)).toBe(defaultValue)
  })
})
