import { headers, type UnsafeUnwrappedHeaders } from 'next/headers'

export function _getAcceptLanguage(header: string | null) {
  // Return empty as default if all languages are accepted
  if (header === '*') {
    return []
  }

  // Split locales by comma
  let locales = header?.split(',')

  // Discart quality parameters
  return locales?.map((l) => l.split(';')[0])
}

export async function getAcceptLanguage() {
  const headersList = await headers()
  return _getAcceptLanguage(headersList.get('Accept-Language'))
}
