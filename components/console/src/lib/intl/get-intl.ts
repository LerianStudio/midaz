'server-only'

import { getCookie, hasCookie } from 'cookies-next/server'
import { createIntl, createIntlCache } from 'react-intl'
import { getIntlConfig } from './get-intl-config'
import { getLocale } from './get-locale'
import { cookies } from 'next/headers'

export async function getIntl() {
  const config = getIntlConfig()

  let locale = ''

  // If the user hasn't set a prefered language yet
  if (await hasCookie('locale', { cookies })) {
    // If the user selected a locale, use it
    locale = (await getCookie('locale', { cookies })) as string
  } else {
    // Set locale by looking for Accept Language header, as system defaults
    // If it fails to find, defaults to I18N default
    locale = await getLocale()
  }

  return createIntl(
    {
      defaultLocale: config.defaultLocale,
      locale: locale,
      messages: (await import(`@/../locales/extracted/${locale}.json`)).default
    },
    createIntlCache()
  )
}
