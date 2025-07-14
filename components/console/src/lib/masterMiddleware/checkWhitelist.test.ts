import { checkWhitelist } from './checkWhitelist'

describe('checkWhitelist', () => {
  const paths = ['/api', '/test']

  test('To be true', () => {
    expect(checkWhitelist('/api', paths)).toBeTruthy()
    expect(checkWhitelist('/api/', paths)).toBeTruthy()
    expect(checkWhitelist('/test', paths)).toBeTruthy()
  })
  test('To be false', () => {
    expect(checkWhitelist('/', paths)).toBeFalsy()
    expect(checkWhitelist('/test2', paths)).toBeFalsy()
  })
})
