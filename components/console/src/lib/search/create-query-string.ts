import { isEmpty, isNil, omitBy } from 'lodash'

/**
 * Function to create a query string from an object.
 * Removes any null, undefined or empty values from the object and
 * automatically adds the '?' at the beginning of the query string.
 * @param data Object containing the query params
 * @returns URL converted query string
 */
export function createQueryString(data?: {}) {
  // Returns empty string if data is null or undefined
  if (isNil(data)) {
    return ''
  }

  // Clear any null, undefined or empty values from the data object
  const clearData = omitBy(omitBy(data, isNil), (v) => v === '')

  // If clearData is empty, there isn't any query params to be added
  if (isEmpty(clearData)) {
    return ''
  }

  const params = new URLSearchParams(Object.entries(clearData))

  return `?${params.toString()}`
}
