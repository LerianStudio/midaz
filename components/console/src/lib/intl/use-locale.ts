import { setCookie } from 'cookies-next'
import { getLocaleCode } from './get-locale-code'
import { useIntl } from 'react-intl'
import { useRouter } from 'next/navigation'

export function useLocale() {
  const intl = useIntl()
  const router = useRouter()

  const setLocale = (value: string) => {
    setCookie('locale', getLocaleCode(value))
    router.refresh()
  }

  return {
    locale: intl.locale,
    setLocale
  }
}
