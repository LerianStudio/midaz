/**
 * Returns the language primary code given an locale string
 * Ex:
 *  pt-BR -> pt |
 *  pt -> pt
 * @param value
 * @returns
 */
export function getLocaleCode(value: string) {
  return value.split('-')[0]
}
