import languages from '.'
import { getIntlConfig } from '../intl'

describe('Languages', () => {
  test('Languages should match the intl configuration settings', () => {
    const intlConfig = getIntlConfig()

    expect(languages).toHaveLength(intlConfig.locales.length)
  })
})
