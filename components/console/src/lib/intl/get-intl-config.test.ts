import { intlConfig } from '@/../intl.config'
import { getIntlConfig } from './get-intl-config'

describe('getIntlConfig', () => {
  test('Should return the intl configuration object', () => {
    expect(getIntlConfig()).toEqual(intlConfig)
  })
})
