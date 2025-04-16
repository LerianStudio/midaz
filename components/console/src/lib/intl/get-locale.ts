import { match } from '@formatjs/intl-localematcher'
import { getAcceptLanguage } from './get-accept-language'
import { getIntlConfig } from './get-intl-config'

/**
 * Matchs the locales available on i18n configuration,
 * against what is available on Accept Language header,
 * If fails, returns the i18n default locale value
 * @returns locale
 */
export function getLocale() {
  const config = getIntlConfig()
  const systemLocales = getAcceptLanguage()

  if (!systemLocales) {
    return config.defaultLocale
  }

  return match(systemLocales, config.locales, config.defaultLocale)
}
