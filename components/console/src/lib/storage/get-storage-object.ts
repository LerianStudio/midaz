import { getStorage } from './get-storage'
import { isNil, isEmpty } from 'lodash'

export function getStorageObject(key: string, defaultValue: any) {
  try {
    const dataString = getStorage(key, defaultValue)
    const isNilOrEmpty = isNil(dataString) || isEmpty(dataString)

    return isNilOrEmpty ? {} : JSON.parse(dataString)
  } catch (error) {
    if (process.env.NODE_ENV !== 'test') {
      console.error(error)
    }
    return defaultValue
  }
}
