import { isNil, pick } from 'lodash'

/**
 * Merge the initial values with the pre-populated data for a form.
 * Data is filtered using the initial values object keys.
 * @param initialValues An object with the initial values of the form
 * @param data An object to pre-populate the form
 * @returns
 */
export function getInitialValues(initialValues?: object, data?: object) {
  if (isNil(initialValues)) {
    return {}
  }

  return {
    ...initialValues,
    ...pick(data, Object.keys(initialValues))
  }
}
