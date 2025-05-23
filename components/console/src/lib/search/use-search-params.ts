'use client'

import {
  useSearchParams as useNextSearchParams,
  usePathname,
  useRouter
} from 'next/navigation'
import { useMemo } from 'react'
import { getSearchParams } from './get-search-params'
import { isNil, omitBy } from 'lodash'
import { createQueryString } from './create-query-string'

/**
 * Wrapper around the next/navigation useSearchParams hook.
 * Provides new functionality and solve some issues from next original implemenation.
 * @returns
 */
export function useSearchParams() {
  const pathname = usePathname()
  const router = useRouter()
  const nextSearchParams = useNextSearchParams()

  /**
   * Sets new search params to the current URL.
   * @param values Object
   */
  const setSearchParams = (values: {}) => {
    const params = omitBy(omitBy(values, isNil), (v) => v === '')

    router.replace(pathname + createQueryString(params))
  }

  /**
   * Updates the current search params with the new values keeping unmodified keys.
   * @param values Object
   */
  const updateSearchParams = (values: {}) => {
    const searchObject = getSearchParams()

    const params = omitBy(
      omitBy({ ...searchObject, ...values }, isNil),
      (v) => v === ''
    )

    router.replace(pathname + createQueryString(params))
  }

  /**
   * Search params provided from browser window object.
   */

  const searchParams = useMemo(() => {
    if (typeof window !== 'undefined') {
      return getSearchParams()
    }
    return null
  }, [typeof window !== 'undefined' ? window.location.search : null])

  return {
    searchParams,
    nextSearchParams,
    setSearchParams,
    updateSearchParams,
    getSearchParams
  }
}
