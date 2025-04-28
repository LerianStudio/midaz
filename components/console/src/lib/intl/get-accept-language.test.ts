import { _getAcceptLanguage } from './get-accept-language'

describe('getAcceptLanguage', () => {
  test('Header with wildcard', () => {
    expect(_getAcceptLanguage('*')).toEqual([])
  })

  test('Header with only one language', () => {
    expect(_getAcceptLanguage('en-US')).toEqual(['en-US'])
  })

  test('Header with multiple language', () => {
    expect(_getAcceptLanguage('pt-BR,en-US,jp-JP')).toEqual([
      'pt-BR',
      'en-US',
      'jp-JP'
    ])
  })

  test('Header with qualities', () => {
    expect(_getAcceptLanguage('pt-BR;q=0.8,en-US;q=0.5')).toEqual([
      'pt-BR',
      'en-US'
    ])
  })
})
