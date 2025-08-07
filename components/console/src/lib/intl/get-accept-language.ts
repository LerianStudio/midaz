import { headers } from 'next/headers'

export function _getAcceptLanguage(header: string | null) {
  if (header === '*') {
    return []
  }

  let locales = header?.split(',')

  return locales?.map((l) => l.split(';')[0])
}

export async function getAcceptLanguage() {
  const headersList = await headers()
  return _getAcceptLanguage(headersList.get('Accept-Language'))
}
