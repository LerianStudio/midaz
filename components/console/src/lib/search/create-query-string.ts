import { isEmpty, isNil, omitBy } from 'lodash'

/**
 * Function to create a query string from an object.
 * Removes any null, undefined or empty values from the object and
 * automatically adds the '?' at the beginning of the query string.
 * @param data Object containing the query params
 * @returns URL converted query string
 */
export function createQueryString(data?: {}) {
  if (isNil(data)) {
    return ''
  }

  const clearData = omitBy(omitBy(data, isNil), (v) => v === '')

  if (isEmpty(clearData)) {
    return ''
  }

  const params = new URLSearchParams(Object.entries(clearData))

  return `?${params.toString()}`
}
