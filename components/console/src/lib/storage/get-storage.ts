const isServer = typeof window === 'undefined'

export function getStorage(key: string, defaultValue: any) {
  if (isServer) {
    return defaultValue
  }

  let value
  try {
    value = localStorage.getItem(key) || undefined
  } catch {}
  return value || defaultValue
}
