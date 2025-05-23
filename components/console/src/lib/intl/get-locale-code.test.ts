import { getLocaleCode } from './get-locale-code'

describe('getLocaleCode', () => {
  it('should return the locale code for a given locale string', () => {
    expect(getLocaleCode('en-US')).toBe('en')
    expect(getLocaleCode('pt-BR')).toBe('pt')
    expect(getLocaleCode('fr-FR')).toBe('fr')
  })

  it('should return the input string if there is no hyphen', () => {
    expect(getLocaleCode('en')).toBe('en')
    expect(getLocaleCode('pt')).toBe('pt')
    expect(getLocaleCode('fr')).toBe('fr')
  })

  it('should return an empty string if the input is an empty string', () => {
    expect(getLocaleCode('')).toBe('')
  })

  it('should handle strings with multiple hyphens correctly', () => {
    expect(getLocaleCode('en-US-California')).toBe('en')
    expect(getLocaleCode('pt-BR-SaoPaulo')).toBe('pt')
  })
})
