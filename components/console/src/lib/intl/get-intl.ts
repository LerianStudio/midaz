'server-only'

import { getCookie, hasCookie } from 'cookies-next/server'
import { createIntl, createIntlCache } from 'react-intl'
import { getIntlConfig } from './get-intl-config'
import { getLocale } from './get-locale'
import { cookies } from 'next/headers'

export async function getIntl() {
  const config = getIntlConfig()

  let locale = ''

  if (await hasCookie('locale', { cookies })) {
    locale = (await getCookie('locale', { cookies })) as string
  } else {
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
